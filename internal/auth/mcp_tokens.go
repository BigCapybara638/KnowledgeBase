package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
)

const mcpTokenPrefix = "kb_mcp_"

type MCPToken struct {
	ID                              domain.ID
	Name                            string
	CreatedAt, ExpiresAt, RevokedAt int64
	CanWrite                        bool
}

func (s *Service) CreateMCPToken(ctx context.Context, actor domain.ID, name string, days int) (MCPToken, string, error) {
	return s.CreateMCPTokenWithWrite(ctx, actor, name, days, false)
}

func (s *Service) CreateMCPTokenWithWrite(ctx context.Context, actor domain.ID, name string, days int, canWrite bool) (MCPToken, string, error) {
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 80 || (days != 30 && days != 90 && days != 365) {
		return MCPToken{}, "", domain.ErrInvalid
	}
	if _, err := s.Store.Editor(ctx, actor); err != nil {
		return MCPToken{}, "", err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return MCPToken{}, "", err
	}
	raw := mcpTokenPrefix + base64.RawURLEncoding.EncodeToString(secret)
	now := s.now()
	key := MCPToken{ID: domain.NewID(), Name: name, CreatedAt: now.Unix(), ExpiresAt: now.Add(time.Duration(days) * 24 * time.Hour).Unix(), CanWrite: canWrite}
	err := s.Store.WithWrite(ctx, func(tx *sql.Tx) error {
		if err := storage.AssertActor(ctx, tx, actor, false); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM mcp_tokens WHERE user_id=? AND revoked_at=0 AND expires_at>?", actor, now.Unix()).Scan(&count); err != nil {
			return err
		}
		if count >= 20 {
			return domain.ErrConflict
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO mcp_tokens(id,user_id,name,token_hash,created_at,expires_at,can_write) VALUES(?,?,?,?,?,?,?)", key.ID, actor, name, tokenHash(raw), key.CreatedAt, key.ExpiresAt, key.CanWrite)
		return err
	})
	if err != nil {
		return MCPToken{}, "", err
	}
	return key, raw, nil
}

func (s *Service) ListMCPTokens(ctx context.Context, actor domain.ID) ([]MCPToken, error) {
	if _, err := s.Store.Editor(ctx, actor); err != nil {
		return nil, err
	}
	rows, err := s.Store.Read.QueryContext(ctx, "SELECT id,name,created_at,expires_at,revoked_at,can_write FROM mcp_tokens WHERE user_id=? ORDER BY (revoked_at=0 AND expires_at>?) DESC,created_at DESC,id DESC LIMIT 100", actor, s.now().Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []MCPToken{}
	for rows.Next() {
		var key MCPToken
		if err := rows.Scan(&key.ID, &key.Name, &key.CreatedAt, &key.ExpiresAt, &key.RevokedAt, &key.CanWrite); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (s *Service) RevokeMCPToken(ctx context.Context, actor, id domain.ID) error {
	return s.Store.WithWrite(ctx, func(tx *sql.Tx) error {
		if err := storage.AssertActor(ctx, tx, actor, false); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, "UPDATE mcp_tokens SET revoked_at=CASE WHEN revoked_at=0 THEN ? ELSE revoked_at END WHERE id=? AND user_id=?", s.now().Unix(), id, actor)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err == nil && n == 0 {
			return domain.ErrNotFound
		}
		return err
	})
}

func (s *Service) ResolveMCPToken(ctx context.Context, raw string) (domain.User, error) {
	access, err := s.ResolveMCPAccess(ctx, raw)
	return access.User, err
}

type MCPAccess struct {
	User     domain.User
	CanWrite bool
}

func (s *Service) ResolveMCPAccess(ctx context.Context, raw string) (MCPAccess, error) {
	if len(raw) != 50 || !strings.HasPrefix(raw, mcpTokenPrefix) {
		return MCPAccess{}, domain.ErrForbidden
	}
	secret, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(raw, mcpTokenPrefix))
	if err != nil || len(secret) != 32 {
		return MCPAccess{}, domain.ErrForbidden
	}
	var access MCPAccess
	u := &access.User
	err = s.Store.Read.QueryRowContext(ctx, `SELECT u.id,u.login,u.name,u.role,u.disabled,u.must_change,t.can_write FROM mcp_tokens t JOIN users u ON u.id=t.user_id WHERE t.token_hash=? AND t.revoked_at=0 AND t.expires_at>?`, tokenHash(raw), s.now().Unix()).Scan(&u.ID, &u.Login, &u.Name, &u.Role, &u.Disabled, &u.MustChangePassword, &access.CanWrite)
	if errors.Is(err, sql.ErrNoRows) || u.Disabled || u.MustChangePassword {
		return MCPAccess{}, domain.ErrForbidden
	}
	if err != nil {
		return MCPAccess{}, err
	}
	return access, nil
}

func (s *Service) SetMCPTokenWrite(ctx context.Context, actor, id domain.ID, canWrite bool) error {
	return s.Store.WithWrite(ctx, func(tx *sql.Tx) error {
		if err := storage.AssertActor(ctx, tx, actor, false); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, "UPDATE mcp_tokens SET can_write=? WHERE id=? AND user_id=? AND revoked_at=0 AND expires_at>?", canWrite, id, actor, s.now().Unix())
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err == nil && n == 0 {
			return domain.ErrNotFound
		}
		return err
	})
}

func revokeMCPKeys(ctx context.Context, tx *sql.Tx, id domain.ID, now int64) error {
	_, err := tx.ExecContext(ctx, "UPDATE mcp_tokens SET revoked_at=? WHERE user_id=? AND revoked_at=0", now, id)
	return err
}
