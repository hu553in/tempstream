package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	HTTPAddr        string          `env:"HTTP_ADDR"                      envDefault:":8080"`
	BaseURL         string          `env:"BASE_URL,required"`
	DBPath          string          `env:"DB_PATH"                        envDefault:"./db.sqlite"`
	TelegramToken   string          `env:"TELEGRAM_BOT_TOKEN,required"`
	AllowedChatIDs  []int64         `env:"ALLOWED_CHAT_IDS,required"`
	MediaHLSBaseURL string          `env:"MEDIAMTX_HLS_BASE_URL,required"`
	CookieSecure    bool            `env:"COOKIE_SECURE"                  envDefault:"true"`
	DefaultLinkTTL  time.Duration   `env:"DEFAULT_LINK_TTL"               envDefault:"1h"`
	LinkTTLOptions  string          `env:"LINK_TTL_OPTIONS"               envDefault:"30m,1h,3h"`
	TimeZone        string          `env:"TIME_ZONE"                      envDefault:"UTC"`
	LogLevel        string          `env:"LOG_LEVEL"                      envDefault:"info"`
	Location        *time.Location  `env:"-"`
	TTLButtons      []time.Duration `env:"-"`
}

func Load() (Config, error) {
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return Config{}, err
	}

	cfg.HTTPAddr = strings.TrimSpace(cfg.HTTPAddr)
	cfg.DBPath = strings.TrimSpace(cfg.DBPath)
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	cfg.MediaHLSBaseURL = strings.TrimRight(cfg.MediaHLSBaseURL, "/") + "/"
	cfg.LinkTTLOptions = strings.TrimSpace(cfg.LinkTTLOptions)
	cfg.TimeZone = strings.TrimSpace(cfg.TimeZone)
	cfg.LogLevel = strings.ToLower(strings.TrimSpace(cfg.LogLevel))

	if cfg.HTTPAddr == "" {
		return Config{}, errors.New("HTTP_ADDR must not be empty")
	}

	if cfg.DBPath == "" {
		return Config{}, errors.New("DB_PATH must not be empty")
	}

	if len(cfg.AllowedChatIDs) == 0 {
		return Config{}, errors.New("ALLOWED_CHAT_IDS must contain at least one chat id")
	}

	if cfg.DefaultLinkTTL < 0 {
		return Config{}, errors.New("DEFAULT_LINK_TTL must be >= 0")
	}

	if cfg.TimeZone == "" {
		return Config{}, errors.New("TIME_ZONE must not be empty")
	}

	location, err := time.LoadLocation(cfg.TimeZone)
	if err != nil {
		return Config{}, fmt.Errorf("TIME_ZONE is invalid: %w", err)
	}
	cfg.Location = location

	ttlButtons, err := parseTTLOptions(cfg.LinkTTLOptions)
	if err != nil {
		return Config{}, err
	}
	cfg.TTLButtons = ttlButtons

	err = validateURL("BASE_URL", cfg.BaseURL)
	if err != nil {
		return Config{}, err
	}

	err = validateURL("MEDIAMTX_HLS_BASE_URL", cfg.MediaHLSBaseURL)
	if err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func validateURL(name, value string) error {
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", name, err)
	}

	if parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%s must be an absolute URL", name)
	}

	return nil
}

func parseTTLOptions(raw string) ([]time.Duration, error) {
	if raw == "" {
		return nil, nil
	}

	parts := strings.Split(raw, ",")
	ttls := make([]time.Duration, 0, len(parts))
	seen := make(map[time.Duration]struct{}, len(parts))

	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}

		ttl, err := time.ParseDuration(value)
		if err != nil {
			return nil, fmt.Errorf("LINK_TTL_OPTIONS contains invalid duration %q: %w", value, err)
		}
		if ttl <= 0 {
			return nil, errors.New("LINK_TTL_OPTIONS must contain only positive durations")
		}
		if _, ok := seen[ttl]; ok {
			continue
		}

		seen[ttl] = struct{}{}
		ttls = append(ttls, ttl)
	}

	return ttls, nil
}
