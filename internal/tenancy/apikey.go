package tenancy

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/S-martin-7/sms/internal/auth"
	sqlcgen "github.com/S-martin-7/sms/internal/db/sqlc/generated"
	"github.com/jackc/pgx/v5"
)

type APIKey struct {
	ID         int64
	TenantID   int64
	Prefix     string
	Name       *string
	Scopes     []string
	LastUsedAt *time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

type IssuedKey struct {
	Token  string // full sk_live_... — show once
	Record *APIKey
}

func (s *Service) IssueAPIKey(ctx context.Context, tenantID int64, name, pepper string) (*IssuedKey, error) {
	if _, err := s.GetTenant(ctx, tenantID); err != nil {
		return nil, err
	}
	token, prefix, err := auth.GenerateToken()
	if err != nil {
		return nil, fmt.Errorf("gen token: %w", err)
	}
	hash := auth.HashToken(token, pepper)
	var namePtr *string
	if name != "" {
		n := name
		namePtr = &n
	}
	row, err := s.q.CreateAPIKey(ctx, sqlcgen.CreateAPIKeyParams{
		TenantID: tenantID,
		Prefix:   prefix,
		Hash:     hash,
		Name:     namePtr,
	})
	if err != nil {
		return nil, fmt.Errorf("insert api_key: %w", err)
	}
	return &IssuedKey{Token: token, Record: apiKeyFromRow(row)}, nil
}

func (s *Service) VerifyAPIKey(ctx context.Context, token, pepper string) (int64, error) {
	prefix, ok := auth.PrefixOf(token)
	if !ok {
		return 0, ErrAPIKeyInvalid
	}
	row, err := s.q.GetAPIKeyByPrefix(ctx, prefix)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrAPIKeyNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("lookup api_key: %w", err)
	}
	if row.RevokedAt.Valid {
		return 0, ErrAPIKeyRevoked
	}
	if !auth.VerifyToken(token, row.Hash, pepper) {
		return 0, ErrAPIKeyInvalid
	}
	go func(id int64) {
		ctxT, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.q.TouchAPIKey(ctxT, id)
	}(row.ID)
	return row.TenantID, nil
}

func (s *Service) RevokeAPIKey(ctx context.Context, id int64) error {
	return s.q.RevokeAPIKey(ctx, id)
}

func (s *Service) ListAPIKeys(ctx context.Context, tenantID int64) ([]*APIKey, error) {
	rows, err := s.q.ListAPIKeysByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list api_keys: %w", err)
	}
	out := make([]*APIKey, 0, len(rows))
	for _, r := range rows {
		out = append(out, apiKeyFromRow(r))
	}
	return out, nil
}

func apiKeyFromRow(r sqlcgen.ApiKey) *APIKey {
	k := &APIKey{
		ID:        r.ID,
		TenantID:  r.TenantID,
		Prefix:    r.Prefix,
		Name:      r.Name,
		Scopes:    r.Scopes,
		CreatedAt: r.CreatedAt.Time,
	}
	if r.LastUsedAt.Valid {
		t := r.LastUsedAt.Time
		k.LastUsedAt = &t
	}
	if r.RevokedAt.Valid {
		t := r.RevokedAt.Time
		k.RevokedAt = &t
	}
	return k
}
