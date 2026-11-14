package auth_test

import (
	"context"
	"errors"
	"github.com/BigCapybara638/KnowledgeBase/internal/auth"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"

	"github.com/BigCapybara638/KnowledgeBase/internal/testutil"
	"strings"
	"sync"
	"testing"
	"time"
)

func fixture(t *testing.T) (*auth.Service, domain.ID) {
	t.Helper()
	s := testutil.Store(t)
	admin := testutil.User(t, s, "admin")
	a, e := auth.New(s, auth.Config{PublicURL: "https://kb.test"})
	if e != nil {
		t.Fatal(e)
	}
	return a, admin
}
func TestTemporaryPasswordRequiresChange(t *testing.T) {
	a, admin := fixture(t)
	u, e := a.CreateUser(context.Background(), admin, auth.NewUser{Login: "Alice", Name: "Алиса", TemporaryPassword: testutil.Password, Role: "user"})
	if e != nil {
		t.Fatal(e)
	}
	if !u.MustChangePassword || u.Login != "alice" {
		t.Fatal(u)
	}
	l, e := a.Login(context.Background(), "ALICE", testutil.Password)
	if e != nil {
		t.Fatal(e)
	}
	if !l.MustChangePassword {
		t.Fatal("missing password gate")
	}
	if e = a.ChangePassword(context.Background(), u.ID, testutil.Password, "Another-password-456!"); e != nil {
		t.Fatal(e)
	}
	if _, e = a.ResolveSession(context.Background(), l.RawToken); !errors.Is(e, domain.ErrForbidden) {
		t.Fatal("old session survived password change")
	}
	l, e = a.Login(context.Background(), "alice", "Another-password-456!")
	if e != nil || l.MustChangePassword {
		t.Fatal(e)
	}
}
func TestNormalRestartKeepsSession(t *testing.T) {
	a, admin := fixture(t)
	u, _ := a.Store.User(context.Background(), admin)
	l, e := a.Login(context.Background(), u.Login, testutil.Password)
	if e != nil {
		t.Fatal(e)
	}
	b, e := auth.New(a.Store, a.Config)
	if e != nil {
		t.Fatal(e)
	}
	got, e := b.ResolveSession(context.Background(), l.RawToken)
	if e != nil || got.ID != admin {
		t.Fatalf("session lost: %v", e)
	}
	if string(a.CSRFKey) != string(b.CSRFKey) || len(a.CSRFKey) != 32 {
		t.Fatal("CSRF key changed")
	}
	if e = b.Logout(context.Background(), l.RawToken); e != nil {
		t.Fatal(e)
	}
	if _, e = b.ResolveSession(context.Background(), l.RawToken); e == nil {
		t.Fatal("logout token still valid")
	}
}
func TestSessionValidForOneWeek(t *testing.T) {
	a, admin := fixture(t)
	ctx := context.Background()
	started := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	clock := started
	a.Config.Now = func() time.Time { return clock }
	u, e := a.Store.User(ctx, admin)
	if e != nil {
		t.Fatal(e)
	}
	l, e := a.Login(ctx, u.Login, testutil.Password)
	if e != nil {
		t.Fatal(e)
	}
	for _, elapsed := range []time.Duration{13 * time.Hour, 7*24*time.Hour - time.Second} {
		clock = started.Add(elapsed)
		got, e := a.ResolveSession(ctx, l.RawToken)
		if e != nil || got.ID != admin {
			t.Fatalf("session invalid after %v: user=%v err=%v", elapsed, got.ID, e)
		}
	}
	clock = started.Add(7 * 24 * time.Hour)
	if _, e := a.ResolveSession(ctx, l.RawToken); !errors.Is(e, domain.ErrForbidden) {
		t.Fatalf("session must expire after one week: %v", e)
	}
}
func TestDisableAndResetRevokeSessions(t *testing.T) {
	a, admin := fixture(t)
	id := testutil.User(t, a.Store, "user")
	u, _ := a.Store.User(context.Background(), id)
	l, e := a.Login(context.Background(), u.Login, testutil.Password)
	if e != nil {
		t.Fatal(e)
	}
	if e = a.SetDisabled(context.Background(), admin, id, true); e != nil {
		t.Fatal(e)
	}
	if _, e = a.ResolveSession(context.Background(), l.RawToken); e == nil {
		t.Fatal("disabled session valid")
	}
	if e = a.SetDisabled(context.Background(), admin, id, false); e != nil {
		t.Fatal(e)
	}
	l, e = a.Login(context.Background(), u.Login, testutil.Password)
	if e != nil {
		t.Fatal(e)
	}
	if e = a.ResetPassword(context.Background(), admin, id, "Reset-password-789!"); e != nil {
		t.Fatal(e)
	}
	if _, e = a.ResolveSession(context.Background(), l.RawToken); e == nil {
		t.Fatal("reset session valid")
	}
}
func TestOnlyAdminCreatesUsers(t *testing.T) {
	a, _ := fixture(t)
	u := testutil.User(t, a.Store, "user")
	_, e := a.CreateUser(context.Background(), u, auth.NewUser{Login: "other", Name: "Other", TemporaryPassword: testutil.Password, Role: "admin"})
	if !errors.Is(e, domain.ErrForbidden) {
		t.Fatalf("non-admin: %v", e)
	}
}
func TestConcurrentDuplicateLogin(t *testing.T) {
	a, admin := fixture(t)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			<-start
			_, e := a.CreateUser(context.Background(), admin, auth.NewUser{Login: "dupe", Name: "Dupe", TemporaryPassword: testutil.Password, Role: "user"})
			errs <- e
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	n := 0
	for e := range errs {
		if e == nil {
			n++
		} else if !errors.Is(e, domain.ErrConflict) {
			t.Fatal(e)
		}
	}
	if n != 1 {
		t.Fatalf("%d duplicate successes", n)
	}
}
func TestLoginLimitPersistsAfterRestart(t *testing.T) {
	a, admin := fixture(t)
	clock := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	a.Config.Now = func() time.Time { return clock }
	u, _ := a.Store.User(context.Background(), admin)
	for range 5 {
		if _, e := a.Login(context.Background(), u.Login, "bad"); e == nil {
			t.Fatal("wrong password accepted")
		}
	}
	b, _ := auth.New(a.Store, a.Config)
	if _, e := b.Login(context.Background(), u.Login, testutil.Password); !errors.Is(e, domain.ErrBusy) {
		t.Fatalf("limit lost: %v", e)
	}
	clock = clock.Add(11 * time.Minute)
	if _, e := b.Login(context.Background(), u.Login, testutil.Password); e != nil {
		t.Fatal(e)
	}
}
func TestPasswordLengthNotTruncated(t *testing.T) {
	a, admin := fixture(t)
	for _, p := range []string{"short", strings.Repeat("a", 73), strings.Repeat("я", 37)} {
		_, e := a.CreateUser(context.Background(), admin, auth.NewUser{Login: "length", Name: "Length", TemporaryPassword: p, Role: "user"})
		if !errors.Is(e, domain.ErrInvalid) {
			t.Fatalf("invalid length: %v", e)
		}
	}
}
func TestReadinessFailsWithClosedDB(t *testing.T) {
	s := testutil.Store(t)
	s.Close()
	if s.Ready(context.Background()) == nil {
		t.Fatal("closed DB ready")
	}
}

func TestRevocationWhileCreateWaitsForWriter(t *testing.T) {
	a, admin := fixture(t)
	tx, e := a.Store.Write.BeginTx(context.Background(), nil)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	before := a.Store.Write.Stats().WaitCount
	done := make(chan error, 1)
	go func() {
		_, e := a.CreateUser(context.Background(), admin, auth.NewUser{Login: "after-revocation", Name: "Blocked", TemporaryPassword: testutil.Password, Role: "user"})
		done <- e
	}()
	deadline := time.Now().Add(8 * time.Second)
	for a.Store.Write.Stats().WaitCount == before && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if a.Store.Write.Stats().WaitCount == before {
		t.Fatal("create did not reach writer queue")
	}
	if _, e = tx.Exec("UPDATE users SET disabled=1 WHERE id=?", admin); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	if e = <-done; !errors.Is(e, domain.ErrForbidden) {
		t.Fatalf("revoked administrator created a user: %v", e)
	}
}
