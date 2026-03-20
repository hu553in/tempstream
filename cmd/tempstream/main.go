package main

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"tempstream/internal/config"
	"tempstream/internal/httpserver"
	"tempstream/internal/logging"
	db "tempstream/internal/repository/sqlc"
	"tempstream/internal/service"
	"tempstream/internal/telegram"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

const (
	outboundHTTPTimeout   = 20 * time.Second
	httpReadTimeout       = 10 * time.Second
	httpReadHeaderTimeout = 10 * time.Second
	httpWriteTimeout      = 120 * time.Second
	httpIdleTimeout       = 120 * time.Second
	shutdownTimeout       = 10 * time.Second
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config error: %w", err)
	}

	log := logging.New(cfg.LogLevel)

	sqlDB, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		return logError(log, "open db failed", err)
	}
	defer sqlDB.Close()

	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	err = ensureDBDir(cfg.DBPath)
	if err != nil {
		log.Error("create db directory failed", slog.String("path", cfg.DBPath), slog.String("err", err.Error()))
		return err
	}

	err = sqlDB.PingContext(ctx)
	if err != nil {
		return logError(log, "ping db failed", err)
	}

	err = runMigrations(sqlDB)
	if err != nil {
		return logError(log, "migrations failed", err)
	}

	queries := db.New(sqlDB)

	httpClient := &http.Client{
		Timeout: outboundHTTPTimeout,
	}

	linkService := service.NewLinkService(
		queries,
		cfg.BaseURL,
		cfg.DefaultLinkTTL,
		httpClient,
		cfg.MediaHLSBaseURL,
	)

	tgBot, err := telegram.New(
		cfg.TelegramToken,
		cfg.AllowedChatIDs,
		linkService,
		cfg.TTLButtons,
		cfg.Location,
		log,
	)
	if err != nil {
		return logError(log, "telegram init failed", err)
	}

	handlers, err := httpserver.NewHandlers(
		log,
		linkService,
		httpClient,
		cfg.MediaHLSBaseURL,
		cfg.CookieSecure,
	)
	if err != nil {
		return logError(log, "http handlers init failed", err)
	}

	router := httpserver.NewRouter(handlers)

	srv := newHTTPServer(cfg.HTTPAddr, log, router)

	go func() {
		log.Info("http server starting", slog.String("addr", cfg.HTTPAddr))
		listenErr := srv.ListenAndServe()
		if listenErr != nil && !errors.Is(listenErr, http.ErrServerClosed) {
			log.Error("http server failed", slog.String("err", listenErr.Error()))
			stop()
		}
	}()

	go tgBot.Start(ctx)

	<-ctx.Done()

	log.Info("shutdown started", slog.String("reason", context.Cause(ctx).Error()))

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	err = srv.Shutdown(shutdownCtx)
	if err != nil {
		log.Error("http shutdown failed", slog.String("err", err.Error()))
	}

	return nil
}

func runMigrations(db *sql.DB) error {
	if err := goose.SetDialect("sqlite"); err != nil {
		return err
	}

	goose.SetBaseFS(migrationsFS)

	return goose.Up(db, "migrations")
}

func ensureDBDir(dbPath string) error {
	dir := filepath.Dir(dbPath)
	if dir == "." || dir == "" {
		return nil
	}

	return os.MkdirAll(dir, 0o750)
}

func accessLog(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(recorder, r)
		log.InfoContext(r.Context(), "http request",
			slog.String("request_id", middleware.GetReqID(r.Context())),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("query", r.URL.RawQuery),
			slog.String("remote_addr", r.RemoteAddr),
			slog.Int("status", recorder.statusCode),
			slog.Int("bytes", recorder.bytesWritten),
			slog.Duration("duration", time.Since(start)),
		)
	})
}

func newHTTPServer(addr string, log *slog.Logger, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           accessLog(log, handler),
		ReadTimeout:       httpReadTimeout,
		ReadHeaderTimeout: httpReadHeaderTimeout,
		WriteTimeout:      httpWriteTimeout,
		IdleTimeout:       httpIdleTimeout,
	}
}

type statusRecorder struct {
	http.ResponseWriter

	statusCode   int
	bytesWritten int
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	written, err := r.ResponseWriter.Write(data)
	r.bytesWritten += written
	return written, err
}

func logError(log *slog.Logger, message string, err error) error {
	log.Error(message, slog.String("err", err.Error()))
	return err
}
