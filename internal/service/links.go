package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	db "tempstream/internal/repository/sqlc"
)

var ErrNotFound = errors.New("not found")

const (
	cleanupDefaultRetention = 7 * 24 * time.Hour
	streamProbeTimeout      = 5 * time.Second
	tokenBytes              = 24
)

type Link struct {
	ID         int64
	Token      string
	Enabled    bool
	CreatedAt  time.Time
	ExpiresAt  *time.Time
	DisabledAt *time.Time
	Note       string
}

type LinkService struct {
	q          *db.Queries
	baseURL    string
	defaultTTL time.Duration
	httpClient *http.Client
	hlsBaseURL string
}

func NewLinkService(
	q *db.Queries,
	baseURL string,
	defaultTTL time.Duration,
	httpClient *http.Client,
	hlsBaseURL string,
) *LinkService {
	return &LinkService{
		q:          q,
		baseURL:    baseURL,
		defaultTTL: defaultTTL,
		httpClient: httpClient,
		hlsBaseURL: hlsBaseURL,
	}
}

func (s *LinkService) CreateLink(ctx context.Context, ttl time.Duration, note string) (Link, string, error) {
	if ttl < 0 {
		return Link{}, "", errors.New("ttl must be >= 0")
	}
	if ttl == 0 {
		ttl = s.defaultTTL
	}

	now := time.Now().UTC()
	nowUnix := now.Unix()
	note = strings.TrimSpace(note)

	token, err := newToken()
	if err != nil {
		return Link{}, "", err
	}

	var expiresAt sql.NullInt64
	if ttl > 0 {
		expiresAt = sql.NullInt64{
			Int64: now.Add(ttl).Unix(),
			Valid: true,
		}
	}

	row, err := s.q.CreateLink(ctx, db.CreateLinkParams{
		Token:     token,
		CreatedAt: nowUnix,
		ExpiresAt: expiresAt,
		Note:      note,
	})
	if err != nil {
		return Link{}, "", err
	}

	link := mapDBLink(row)
	return link, s.WatchURL(link.Token), nil
}

func (s *LinkService) CreatePermanentLink(ctx context.Context, note string) (Link, string, error) {
	nowUnix := time.Now().UTC().Unix()
	note = strings.TrimSpace(note)

	token, err := newToken()
	if err != nil {
		return Link{}, "", err
	}

	row, err := s.q.CreateLink(ctx, db.CreateLinkParams{
		Token:     token,
		CreatedAt: nowUnix,
		ExpiresAt: sql.NullInt64{},
		Note:      note,
	})
	if err != nil {
		return Link{}, "", err
	}

	link := mapDBLink(row)
	return link, s.WatchURL(link.Token), nil
}

func (s *LinkService) DisableByID(ctx context.Context, id int64) (Link, error) {
	if id <= 0 {
		return Link{}, ErrNotFound
	}

	nowUnix := time.Now().UTC().Unix()

	affected, err := s.q.DisableLinkByID(ctx, db.DisableLinkByIDParams{
		DisabledAt: sql.NullInt64{Int64: nowUnix, Valid: true},
		ID:         id,
	})
	if err != nil {
		return Link{}, err
	}
	if affected == 0 {
		return Link{}, ErrNotFound
	}

	row, err := s.q.GetLinkByID(ctx, id)
	if err != nil {
		return Link{}, err
	}

	return mapDBLink(row), nil
}

func (s *LinkService) DisableLast(ctx context.Context) (Link, error) {
	nowUnix := time.Now().UTC().Unix()

	row, err := s.q.GetLastActiveLink(ctx, sql.NullInt64{Int64: nowUnix, Valid: true})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Link{}, ErrNotFound
		}
		return Link{}, err
	}

	return s.DisableByID(ctx, row.ID)
}

func (s *LinkService) ListActive(ctx context.Context) ([]Link, error) {
	nowUnix := time.Now().UTC().Unix()

	rows, err := s.q.ListActiveLinks(ctx, sql.NullInt64{Int64: nowUnix, Valid: true})
	if err != nil {
		return nil, err
	}

	out := make([]Link, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapDBLink(row))
	}

	return out, nil
}

func (s *LinkService) ValidateToken(ctx context.Context, token string) (Link, error) {
	token = strings.TrimSpace(token)
	if !isValidToken(token) {
		return Link{}, ErrNotFound
	}

	nowUnix := time.Now().UTC().Unix()

	row, err := s.q.GetActiveLinkByToken(ctx, db.GetActiveLinkByTokenParams{
		Token:     token,
		ExpiresAt: sql.NullInt64{Int64: nowUnix, Valid: true},
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Link{}, ErrNotFound
		}
		return Link{}, err
	}

	return mapDBLink(row), nil
}

func (s *LinkService) WatchURL(token string) string {
	return s.baseURL + "/live/stream/" + token
}

func (s *LinkService) StreamLooksAlive(ctx context.Context) bool {
	probeCtx, cancel := context.WithTimeout(ctx, streamProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, s.hlsBaseURL+"index.m3u8", nil)
	if err != nil {
		return false
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func (s *LinkService) Cleanup(ctx context.Context, olderThan time.Duration) (int64, error) {
	if olderThan <= 0 {
		olderThan = cleanupDefaultRetention
	}

	cutoffUnix := time.Now().UTC().Add(-olderThan).Unix()
	return s.q.DeleteExpiredDisabledLinks(ctx, sql.NullInt64{Int64: cutoffUnix, Valid: true})
}

func mapDBLink(row db.WatchLink) Link {
	link := Link{
		ID:        row.ID,
		Token:     row.Token,
		Enabled:   row.Enabled == 1,
		CreatedAt: time.Unix(row.CreatedAt, 0).UTC(),
		Note:      row.Note,
	}

	if row.ExpiresAt.Valid {
		t := time.Unix(row.ExpiresAt.Int64, 0).UTC()
		link.ExpiresAt = &t
	}

	if row.DisabledAt.Valid {
		t := time.Unix(row.DisabledAt.Int64, 0).UTC()
		link.DisabledAt = &t
	}

	return link
}

func newToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func isValidToken(token string) bool {
	if len(token) == 0 || len(token) > 128 {
		return false
	}

	for _, r := range token {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}

	return true
}
