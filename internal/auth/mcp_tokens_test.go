package auth_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/testutil"
)

func TestMCPKeyLifecycle(t *testing.T) {
	a, actor := fixture(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)
	a.Config.Now = func() time.Time { return now }
	key, raw, err := a.CreateMCPToken(ctx, actor, "  Рабочий агент  ", 90)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 50 || !strings.HasPrefix(raw, "kb_mcp_") || key.Name != "Рабочий агент" || key.ExpiresAt != now.Add(90*24*time.Hour).Unix() {
		t.Fatalf("bad key metadata: %+v", key)
	}
	var stored string
	if err := a.Store.Read.QueryRow("SELECT token_hash FROM mcp_tokens WHERE id=?", key.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(raw))
	if stored != hex.EncodeToString(digest[:]) || stored == raw {
		t.Fatal("raw key persisted or incorrect hash")
	}
	u, err := a.ResolveMCPToken(ctx, raw)
	if err != nil || u.ID != actor {
		t.Fatalf("resolve: %v", err)
	}
	other := testutil.User(t, a.Store, "user")
	if err := a.RevokeMCPToken(ctx, other, key.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign revoke: %v", err)
	}
	if keys, err := a.ListMCPTokens(ctx, other); err != nil || len(keys) != 0 {
		t.Fatalf("foreign list: %v", err)
	}
	if err := a.RevokeMCPToken(ctx, actor, key.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ResolveMCPToken(ctx, raw); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("revoked key accepted")
	}
	keys, err := a.ListMCPTokens(ctx, actor)
	if err != nil || len(keys) != 1 || keys[0].RevokedAt == 0 {
		t.Fatalf("list: %+v %v", keys, err)
	}
}

func TestMCPKeyExpiryAndAccountGate(t *testing.T) {
	a, admin := fixture(t)
	ctx := context.Background()
	id := testutil.User(t, a.Store, "user")
	now := time.Now().Truncate(time.Second)
	a.Config.Now = func() time.Time { return now }
	_, raw, err := a.CreateMCPToken(ctx, id, "Agent", 30)
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"disabled", "must_change"} {
		if _, err := a.Store.Write.Exec("UPDATE users SET "+column+"=1 WHERE id=?", id); err != nil {
			t.Fatal(err)
		}
		if _, err := a.ResolveMCPToken(ctx, raw); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("%s key accepted", column)
		}
		if _, _, err := a.CreateMCPToken(ctx, id, "Forbidden", 30); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("%s issuance accepted", column)
		}
		if _, err := a.Store.Write.Exec("UPDATE users SET "+column+"=0 WHERE id=?", id); err != nil {
			t.Fatal(err)
		}
	}
	now = now.Add(30 * 24 * time.Hour)
	if _, err := a.ResolveMCPToken(ctx, raw); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("expired key accepted")
	}
	for _, raw := range []string{"", strings.Repeat("x", 50), "kb_mcp_" + strings.Repeat("a", 43)} {
		if _, err := a.ResolveMCPToken(ctx, raw); !errors.Is(err, domain.ErrForbidden) {
			t.Fatal("invalid key accepted")
		}
	}
	if err := a.SetDisabled(ctx, admin, id, true); err != nil {
		t.Fatal(err)
	}
}

func TestMCPKeyValidationAndLimit(t *testing.T) {
	a, actor := fixture(t)
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		days int
	}{{"", 90}, {"  ", 90}, {strings.Repeat("я", 81), 90}, {"Agent", 1}, {"Agent", 0}} {
		if _, _, err := a.CreateMCPToken(ctx, actor, tc.name, tc.days); !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("invalid key input accepted: %+v %v", tc, err)
		}
	}
	for range 19 {
		if _, _, err := a.CreateMCPToken(ctx, actor, "Agent", 90); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Go(func() { _, _, err := a.CreateMCPToken(ctx, actor, "Last", 90); errs <- err })
	}
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		} else if !errors.Is(err, domain.ErrConflict) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatalf("limit race: %d successes", success)
	}
}

func TestMCPKeysRevokedByPasswordChanges(t *testing.T) {
	for _, reset := range []bool{false, true} {
		t.Run(map[bool]string{false: "change", true: "reset"}[reset], func(t *testing.T) {
			a, admin := fixture(t)
			ctx := context.Background()
			id := testutil.User(t, a.Store, "user")
			_, raw, err := a.CreateMCPToken(ctx, id, "Agent", 90)
			if err != nil {
				t.Fatal(err)
			}
			if reset {
				err = a.ResetPassword(ctx, admin, id, "Changed-password-456!")
			} else {
				err = a.ChangePassword(ctx, id, testutil.Password, "Changed-password-456!")
			}
			if err != nil {
				t.Fatal(err)
			}
			if reset {
				if _, err := a.Store.Write.Exec("UPDATE users SET must_change=0 WHERE id=?", id); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := a.ResolveMCPToken(ctx, raw); !errors.Is(err, domain.ErrForbidden) {
				t.Fatal("old key survived password change")
			}
		})
	}
}
