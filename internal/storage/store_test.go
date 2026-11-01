package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, e := Open(context.Background(), t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func TestOpenEnforcesPragmasOnEveryConnection(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	var cs []*sql.Conn
	for range 4 {
		c, e := s.Read.Conn(ctx)
		if e != nil {
			t.Fatal(e)
		}
		cs = append(cs, c)
		var fk, busy int
		var mode string
		c.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk)
		c.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busy)
		c.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode)
		if fk != 1 || busy != 5000 || mode != "wal" {
			t.Fatalf("unsafe settings: fk=%d timeout=%d journal=%s", fk, busy, mode)
		}
	}
	for _, c := range cs {
		c.Close()
	}
}
func TestMigrationsRepeatable(t *testing.T) {
	d := t.TempDir()
	s, e := Open(context.Background(), d)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Write.Exec("INSERT INTO users(id,login,name,password_hash,role,must_change) VALUES('u','test','Test','hash','admin',1)")
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = Open(context.Background(), d)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	var n int
	s.Read.QueryRow("SELECT count(*) FROM users WHERE id='u'").Scan(&n)
	if n != 1 {
		t.Fatalf("lost data: %d", n)
	}
}
func TestSecondServeRejected(t *testing.T) {
	d := t.TempDir()
	a, e := AcquireInstanceLock(d)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	if b, e := AcquireInstanceLock(d); e == nil {
		b.Close()
		t.Fatal("second server accepted")
	}
	if filepath.Base(d) == "" {
		t.Fatal("invalid temp dir")
	}
}
func TestReadinessFailsWithClosedDB(t *testing.T) {
	s := openTest(t)
	s.Close()
	if s.Ready(context.Background()) == nil {
		t.Fatal("closed database reported ready")
	}
}
