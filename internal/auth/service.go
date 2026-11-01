package auth

import (
	"context"
	"database/sql"

	"regexp"
	"strings"
	"time"

	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

type Config struct {
	PublicURL string
	Dev       bool
	Now       func() time.Time
}
type NewUser struct{ Login, Name, TemporaryPassword, Role string }
type LoginResult struct {
	RawToken           string
	MustChangePassword bool
}
type Service struct {
	Store   *storage.Store
	Config  Config
	CSRFKey []byte
	slots   chan struct{}
}

var validLogin = regexp.MustCompile(`^[a-z0-9][a-z0-9._@+-]{2,63}$`)

func New(s *storage.Store, c Config) (*Service, error) {
	key, e := loadKey(s.DataDir)
	if e != nil {
		return nil, e
	}
	return &Service{Store: s, Config: c, CSRFKey: key, slots: make(chan struct{}, 2)}, nil
}
func (s *Service) now() time.Time {
	if s.Config.Now != nil {
		return s.Config.Now()
	}
	return time.Now()
}
func normalize(login string) string { return strings.ToLower(strings.TrimSpace(login)) }
func (s *Service) CreateUser(ctx context.Context, actor domain.ID, in NewUser) (domain.User, error) {
	if e := s.Store.Admin(ctx, actor); e != nil {
		return domain.User{}, e
	}
	return s.create(ctx, in, false)
}
func (s *Service) Bootstrap(ctx context.Context, in NewUser) (domain.User, error) {
	in.Role = "admin"
	return s.create(ctx, in, true)
}
func (s *Service) create(ctx context.Context, in NewUser, bootstrap bool) (domain.User, error) {
	in.Login = normalize(in.Login)
	in.Name = strings.TrimSpace(in.Name)
	if !validLogin.MatchString(in.Login) || in.Name == "" || len(in.Name) > 256 || (in.Role != "admin" && in.Role != "user") {
		return domain.User{}, domain.ErrInvalid
	}
	h, e := passwordHash(in.TemporaryPassword)
	if e != nil {
		return domain.User{}, e
	}
	u := domain.User{ID: domain.NewID(), Login: in.Login, Name: in.Name, Role: in.Role, MustChangePassword: true}
	e = s.Store.WithWrite(ctx, func(tx *sql.Tx) error {
		if bootstrap {
			var n int
			if e := tx.QueryRowContext(ctx, "SELECT count(*) FROM users").Scan(&n); e != nil {
				return e
			}
			if n != 0 {
				return domain.ErrForbidden
			}
		}
		_, e := tx.ExecContext(ctx, "INSERT INTO users(id,login,name,password_hash,role,must_change) VALUES(?,?,?,?,?,1)", u.ID, u.Login, u.Name, h, u.Role)
		return e
	})
	return u, e
}
func (s *Service) ChangePassword(ctx context.Context, id domain.ID, old, newPassword string) error {
	if _, e := s.Store.User(ctx, id); e != nil {
		return e
	}
	var hash []byte
	if e := s.Store.Read.QueryRowContext(ctx, "SELECT password_hash FROM users WHERE id=?", id).Scan(&hash); e != nil {
		return e
	}
	if e := bcrypt.CompareHashAndPassword(hash, []byte(old)); e != nil {
		return domain.ErrForbidden
	}
	newHash, e := passwordHash(newPassword)
	if e != nil {
		return e
	}
	return s.Store.WithWrite(ctx, func(tx *sql.Tx) error {
		r, e := tx.ExecContext(ctx, "UPDATE users SET password_hash=?,must_change=0 WHERE id=? AND password_hash=? AND disabled=0", newHash, id, hash)
		if e != nil {
			return e
		}
		n, _ := r.RowsAffected()
		if n != 1 {
			return domain.ErrConflict
		}
		_, e = tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id=?", id)
		return e
	})
}
func (s *Service) ResetPassword(ctx context.Context, admin, id domain.ID, password string) error {
	if e := s.Store.Admin(ctx, admin); e != nil {
		return e
	}
	hash, e := passwordHash(password)
	if e != nil {
		return e
	}
	return s.Store.WithWrite(ctx, func(tx *sql.Tx) error {
		r, e := tx.ExecContext(ctx, "UPDATE users SET password_hash=?,must_change=1 WHERE id=?", hash, id)
		if e != nil {
			return e
		}
		n, _ := r.RowsAffected()
		if n == 0 {
			return domain.ErrNotFound
		}
		if _, e = tx.ExecContext(ctx, "DELETE FROM login_limits WHERE login=(SELECT login FROM users WHERE id=?)", id); e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id=?", id)
		return e
	})
}
func (s *Service) SetDisabled(ctx context.Context, admin, id domain.ID, disabled bool) error {
	if e := s.Store.Admin(ctx, admin); e != nil {
		return e
	}
	if id == admin && disabled {
		return domain.ErrInvalid
	}
	return s.Store.WithWrite(ctx, func(tx *sql.Tx) error {
		r, e := tx.ExecContext(ctx, "UPDATE users SET disabled=? WHERE id=?", disabled, id)
		if e != nil {
			return e
		}
		n, _ := r.RowsAffected()
		if n != 1 {
			return domain.ErrNotFound
		}
		_, e = tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id=?", id)
		return e
	})
}
func (s *Service) Users(ctx context.Context, admin domain.ID) ([]domain.User, error) {
	if e := s.Store.Admin(ctx, admin); e != nil {
		return nil, e
	}
	rs, e := s.Store.Read.QueryContext(ctx, "SELECT id,login,name,role,disabled,must_change FROM users ORDER BY name,id")
	if e != nil {
		return nil, e
	}
	defer rs.Close()
	out := []domain.User{}
	for rs.Next() {
		var u domain.User
		if e = rs.Scan(&u.ID, &u.Login, &u.Name, &u.Role, &u.Disabled, &u.MustChangePassword); e != nil {
			return nil, e
		}
		out = append(out, u)
	}
	return out, rs.Err()
}
