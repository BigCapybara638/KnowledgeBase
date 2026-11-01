package storage

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/driver"
	"github.com/ncruces/go-sqlite3/ext/fts5"
	"github.com/pressly/goose/v3"
	"golang.org/x/sys/unix"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	restoreLease io.Closer
	Read, Write  *sql.DB
	DataDir      string
}

func Open(ctx context.Context, dataDir string) (*Store, error) {
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(abs, 0700); err != nil {
		return nil, err
	}
	lease, err := AcquireFileLock(abs, "restore.lock", false, true)
	if err != nil {
		return nil, err
	}
	leaseOwned := true
	defer func() {
		if leaseOwned {
			lease.Close()
		}
	}()
	if _, e := os.Stat(filepath.Join(abs, "restore-in-progress")); e == nil {
		return nil, errors.New("incomplete restore: preserve the directory and restore into an empty one")
	}
	u := url.URL{Scheme: "file", Path: filepath.Join(abs, "knowledgebase.db")}
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "cache_size(-4096)")
	q.Set("_txlock", "immediate")
	u.RawQuery = q.Encode()
	w, err := driver.Open(u.String(), fts5.Register)
	if err != nil {
		return nil, err
	}
	w.SetMaxOpenConns(1)
	w.SetMaxIdleConns(1)
	s := &Store{Write: w, DataDir: abs, restoreLease: lease}
	leaseOwned = false
	ok := false
	defer func() {
		if !ok {
			s.Close()
		}
	}()
	if err = w.PingContext(ctx); err != nil {
		return nil, err
	}
	if err = os.Chmod(filepath.Join(abs, "knowledgebase.db"), 0600); err != nil {
		return nil, err
	}
	sub, _ := fs.Sub(migrations, "migrations")
	p, err := goose.NewProvider(goose.DialectSQLite3, w, sub, goose.WithDisableGlobalRegistry(true))
	if err != nil {
		return nil, err
	}
	if _, err = p.Up(ctx); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	q.Set("_txlock", "deferred")
	q.Add("_pragma", "query_only(1)")
	u.RawQuery = q.Encode()
	r, err := driver.Open(u.String(), fts5.Register)
	if err != nil {
		return nil, err
	}
	r.SetMaxOpenConns(4)
	r.SetMaxIdleConns(4)
	s.Read = r
	if err = r.PingContext(ctx); err != nil {
		return nil, err
	}
	ok = true
	return s, nil
}
func (s *Store) Close() error {
	var a, b error
	if s.Read != nil {
		a = s.Read.Close()
	}
	if s.Write != nil {
		b = s.Write.Close()
	}
	if s.restoreLease != nil {
		s.restoreLease.Close()
	}
	return errors.Join(a, b)
}
func (s *Store) Ready(ctx context.Context) error {
	var n int
	return s.Read.QueryRowContext(ctx, "SELECT count(*) FROM goose_db_version").Scan(&n)
}
func (s *Store) WithWrite(ctx context.Context, fn func(*sql.Tx) error) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, e := s.Write.BeginTx(ctx, nil)
	if e != nil {
		return mapError(e)
	}
	defer tx.Rollback()
	if e = fn(tx); e != nil {
		return mapError(e)
	}
	return mapError(tx.Commit())
}
func mapError(e error) error {
	if e == nil {
		return nil
	}
	if errors.Is(e, sqlite3.BUSY) || errors.Is(e, sqlite3.LOCKED) || errors.Is(e, context.DeadlineExceeded) {
		return domain.ErrBusy
	}
	if errors.Is(e, sqlite3.CONSTRAINT_UNIQUE) {
		return domain.ErrConflict
	}
	return e
}
func (s *Store) User(ctx context.Context, id domain.ID) (domain.User, error) {
	var u domain.User
	e := s.Read.QueryRowContext(ctx, "SELECT id,login,name,role,disabled,must_change FROM users WHERE id=?", id).Scan(&u.ID, &u.Login, &u.Name, &u.Role, &u.Disabled, &u.MustChangePassword)
	if errors.Is(e, sql.ErrNoRows) || u.Disabled {
		return u, domain.ErrForbidden
	}
	return u, e
}
func (s *Store) Editor(ctx context.Context, id domain.ID) (domain.User, error) {
	u, e := s.User(ctx, id)
	if e == nil && u.MustChangePassword {
		e = domain.ErrForbidden
	}
	return u, e
}
func (s *Store) Admin(ctx context.Context, id domain.ID) error {
	u, e := s.Editor(ctx, id)
	if e != nil {
		return e
	}
	if u.Role != "admin" {
		return domain.ErrForbidden
	}
	return nil
}
func AcquireInstanceLock(dataDir string) (io.Closer, error) {
	return AcquireFileLock(dataDir, "instance.lock", true, true)
}
func AcquireFileLock(dir, name string, exclusive, nonblocking bool) (io.Closer, error) {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	op := unix.LOCK_SH
	if exclusive {
		op = unix.LOCK_EX
	}
	if nonblocking {
		op |= unix.LOCK_NB
	}
	if e = unix.Flock(int(f.Fd()), op); e != nil {
		f.Close()
		return nil, fmt.Errorf("resource in use: %w", e)
	}
	return f, nil
}

// AssertActor runs inside a write transaction so revocation cannot race a write.
func AssertActor(ctx context.Context, tx *sql.Tx, id domain.ID, admin bool) error {
	var role string
	var disabled, mustChange bool
	e := tx.QueryRowContext(ctx, "SELECT role,disabled,must_change FROM users WHERE id=?", id).Scan(&role, &disabled, &mustChange)
	if errors.Is(e, sql.ErrNoRows) || disabled || mustChange || (admin && role != "admin") {
		return domain.ErrForbidden
	}
	return e
}

// OpenExisting never creates or migrates a database. Callers coordinate restore locks.
func OpenExisting(ctx context.Context, dir string, readOnly bool) (*sql.DB, error) {
	p, e := filepath.Abs(filepath.Join(dir, "knowledgebase.db"))
	if e != nil {
		return nil, e
	}
	info, e := os.Lstat(p)
	if e != nil {
		return nil, e
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("database must be a regular file")
	}
	u := url.URL{Scheme: "file", Path: p}
	q := url.Values{}
	q.Set("mode", "rw")
	if readOnly {
		q.Set("mode", "ro")
	}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "cache_size(-4096)")
	u.RawQuery = q.Encode()
	db, e := driver.Open(u.String(), fts5.Register)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	if e = db.PingContext(ctx); e != nil {
		db.Close()
		return nil, e
	}
	return db, nil
}
