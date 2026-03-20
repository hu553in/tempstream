package httpserver

import (
	"embed"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"

	"tempstream/internal/service"
)

//go:embed templates/watch.html
var templatesFS embed.FS

const (
	watchSessionCookieName = "watch_session"
	playlistPath           = "/play/index.m3u8"
	errAccessDenied        = "access denied"
	errInternal            = "internal error"
	errMethodNotAllowed    = "method not allowed"
	errStreamUnavailable   = "stream unavailable"
	errInvalidLink         = "link is invalid or expired"
	requestHeaderCapacity  = 6
)

type Handlers struct {
	log           *slog.Logger
	links         *service.LinkService
	mediaBaseURL  *url.URL
	cookieSecure  bool
	playProxy     *httputil.ReverseProxy
	watchPageTmpl *template.Template
}

func NewHandlers(
	log *slog.Logger,
	links *service.LinkService,
	httpClient *http.Client,
	mediaBaseURL string,
	cookieSecure bool,
) (*Handlers, error) {
	mediaURL, err := url.Parse(mediaBaseURL)
	if err != nil {
		return nil, err
	}

	watchPageTmpl, err := template.ParseFS(templatesFS, "templates/watch.html")
	if err != nil {
		return nil, err
	}

	playProxy := httputil.NewSingleHostReverseProxy(mediaURL)
	playProxy.Transport = httpClient.Transport
	playProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, proxyErr error) {
		log.WarnContext(r.Context(), "stream proxy request failed",
			slog.String("path", r.URL.Path),
			slog.String("err", proxyErr.Error()),
		)
		writePlainError(w, http.StatusBadGateway, errStreamUnavailable)
	}
	playProxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Set("Cache-Control", "no-store, private")
		resp.Header.Del("Access-Control-Allow-Origin")
		return nil
	}

	return &Handlers{
		log:           log,
		links:         links,
		mediaBaseURL:  mediaURL,
		cookieSecure:  cookieSecure,
		playProxy:     playProxy,
		watchPageTmpl: watchPageTmpl,
	}, nil
}

func (h *Handlers) Index(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("watch service is running"))
}

func (h *Handlers) Healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}

func (h *Handlers) WatchPage(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(chi.URLParam(r, "token"))
	if token == "" {
		http.NotFound(w, r)
		return
	}

	link, err := h.links.ValidateToken(r.Context(), token)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			writePlainError(w, http.StatusForbidden, errInvalidLink)
			return
		}

		h.log.ErrorContext(r.Context(), "validate watch token failed", slog.String("err", err.Error()))
		writePlainError(w, http.StatusInternalServerError, errInternal)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     watchSessionCookieName,
		Value:    link.Token,
		Path:     "/play/",
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   0,
	})

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	executeErr := h.watchPageTmpl.Execute(w, struct {
		PlaylistURL string
	}{
		PlaylistURL: playlistPath,
	})
	if executeErr != nil {
		h.log.ErrorContext(r.Context(), "render watch page failed", slog.String("err", executeErr.Error()))
	}
}

func (h *Handlers) PlayProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writePlainError(w, http.StatusMethodNotAllowed, errMethodNotAllowed)
		return
	}

	cookie, err := r.Cookie(watchSessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		writePlainError(w, http.StatusForbidden, errAccessDenied)
		return
	}

	_, validateErr := h.links.ValidateToken(r.Context(), cookie.Value)
	if validateErr != nil {
		clearWatchCookie(w, h.cookieSecure)
		writePlainError(w, http.StatusForbidden, errAccessDenied)
		return
	}

	relPath, ok := sanitizePlayPath(strings.TrimPrefix(r.URL.Path, "/play/"))
	if !ok {
		http.NotFound(w, r)
		return
	}

	proxyReq := r.Clone(r.Context())
	proxyReq.URL = cloneURL(r.URL)
	proxyReq.URL.Path = "/" + relPath
	proxyReq.URL.RawQuery = r.URL.RawQuery
	proxyReq.Host = ""
	proxyReq.RequestURI = ""
	proxyReq.Header = copyRequestHeaders(r.Header)

	h.playProxy.ServeHTTP(w, proxyReq)
}

func copyRequestHeaders(src http.Header) http.Header {
	dst := make(http.Header, requestHeaderCapacity)

	for _, key := range []string{"Accept", "If-Modified-Since", "If-None-Match", "If-Range", "Range", "User-Agent"} {
		for _, value := range src.Values(key) {
			dst.Add(key, value)
		}
	}

	return dst
}

func clearWatchCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     watchSessionCookieName,
		Value:    "",
		Path:     "/play/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

func cloneURL(src *url.URL) *url.URL {
	if src == nil {
		return &url.URL{}
	}

	cloned := *src
	return &cloned
}

func sanitizePlayPath(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, "\\") {
		return "", false
	}

	cleaned := path.Clean("/" + raw)
	if cleaned == "/" {
		return "", false
	}

	trimmed := strings.TrimPrefix(cleaned, "/")
	if strings.HasPrefix(trimmed, "..") || strings.Contains(trimmed, "/..") {
		return "", false
	}

	return trimmed, true
}

func writePlainError(w http.ResponseWriter, statusCode int, message string) {
	http.Error(w, message, statusCode)
}
