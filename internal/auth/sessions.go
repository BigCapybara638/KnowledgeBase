package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage/dbgen"
	"golang.org/x/crypto/bcrypt"
	"time"
)

// SessionLifetime is the absolute validity period from login, without renewal on requests.
const SessionLifetime = 7 * 24 * time.Hour

func tokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
func (s *Service) Login(ctx context.Context, login, password string) (LoginResult, error) {
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return LoginResult{}, domain.ErrBusy
	}
	login = normalize(login)
	if !validLogin.MatchString(login) || len(password) > 72 {
		return LoginResult{}, domain.ErrForbidden
	}
	var id domain.ID
	var hash []byte
	var disabled, mustChange bool
	e := s.Store.Read.QueryRowContext(ctx, "SELECT id,password_hash,disabled,must_change FROM users WHERE login=?", login).Scan(&id, &hash, &disabled, &mustChange)
	if errors.Is(e, sql.ErrNoRows) {
		return LoginResult{}, domain.ErrForbidden
	}
	if e != nil {
		return LoginResult{}, e
	}
	now := s.now().Unix()
	e = s.Store.WithWrite(ctx, func(tx *sql.Tx) error {
		var n int
		var started int64
		e := tx.QueryRowContext(ctx, "SELECT failures,window_start FROM login_limits WHERE login=?", login).Scan(&n, &started)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if now-started >= 600 {
			n = 0
			started = now
		}
		if n >= 5 {
			return domain.ErrBusy
		}
		_, e = tx.ExecContext(ctx, "INSERT INTO login_limits(login,failures,window_start) VALUES(?,?,?) ON CONFLICT(login) DO UPDATE SET failures=excluded.failures,window_start=excluded.window_start", login, n+1, started)
		return e
	})
	if e != nil {
		return LoginResult{}, e
	}
	if e = bcrypt.CompareHashAndPassword(hash, []byte(password)); e != nil || disabled {
		return LoginResult{}, domain.ErrForbidden
	}
	raw := make([]byte, 32)
	if _, e = rand.Read(raw); e != nil {
		return LoginResult{}, e
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	e = s.Store.WithWrite(ctx, func(tx *sql.Tx) error {
		var current []byte
		var inactive bool
		if e := tx.QueryRowContext(ctx, "SELECT password_hash,disabled FROM users WHERE id=?", id).Scan(&current, &inactive); e != nil {
			return e
		}
		if inactive || string(current) != string(hash) {
			return domain.ErrForbidden
		}
		if _, e := tx.ExecContext(ctx, "DELETE FROM login_limits WHERE login=?", login); e != nil {
			return e
		}
		_, e := tx.ExecContext(ctx, "INSERT INTO sessions(token_hash,user_id,expires_at) VALUES(?,?,?)", tokenHash(token), id, now+int64(SessionLifetime/time.Second))
		return e
	})
	return LoginResult{RawToken: token, MustChangePassword: mustChange}, e
}
func (s *Service) ResolveSession(ctx context.Context, raw string) (domain.User, error) {
	if len(raw) != 43 {
		return domain.User{}, domain.ErrForbidden
	}
	r, e := dbgen.New(s.Store.Read).SessionUser(ctx, dbgen.SessionUserParams{TokenHash: tokenHash(raw), ExpiresAt: s.now().Unix()})
	if errors.Is(e, sql.ErrNoRows) || r.Disabled != 0 {
		return domain.User{}, domain.ErrForbidden
	}
	return domain.User{ID: domain.ID(r.ID), Login: r.Login, Name: r.Name, Role: r.Role, Disabled: r.Disabled != 0, MustChangePassword: r.MustChange != 0}, e
}
func (s *Service) Logout(ctx context.Context, raw string) error {
	return s.Store.WithWrite(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash=?", tokenHash(raw))
		return e
	})
}
