package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/BigCapybara638/KnowledgeBase/internal/auth"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"github.com/BigCapybara638/KnowledgeBase/internal/testutil"
)

func TestMCPKeyWritePermissionIsExplicitAndOwned(t *testing.T) {
	a, actor := fixture(t)
	ctx := context.Background()
	key, raw, err := a.CreateMCPToken(ctx, actor, "Reader", 90)
	if err != nil {
		t.Fatal(err)
	}
	access, err := a.ResolveMCPAccess(ctx, raw)
	if err != nil || access.CanWrite || key.CanWrite || access.User.ID != actor {
		t.Fatalf("default access: %+v %v", access, err)
	}
	other := testutil.User(t, a.Store, "user")
	if err = a.SetMCPTokenWrite(ctx, other, key.ID, true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign permission change: %v", err)
	}
	if err = a.SetMCPTokenWrite(ctx, actor, key.ID, true); err != nil {
		t.Fatal(err)
	}
	access, err = a.ResolveMCPAccess(ctx, raw)
	if err != nil || !access.CanWrite {
		t.Fatalf("permission not applied to existing secret: %v", err)
	}
	keys, err := a.ListMCPTokens(ctx, actor)
	if err != nil || len(keys) != 1 || !keys[0].CanWrite {
		t.Fatalf("key list: %+v %v", keys, err)
	}
	if err = a.SetMCPTokenWrite(ctx, actor, key.ID, false); err != nil {
		t.Fatal(err)
	}
	access, err = a.ResolveMCPAccess(ctx, raw)
	if err != nil || access.CanWrite {
		t.Fatal("permission not removed")
	}
	written, wraw, err := a.CreateMCPTokenWithWrite(ctx, actor, "Writer", 30, true)
	if err != nil || !written.CanWrite {
		t.Fatalf("write key creation: %v", err)
	}
	access, err = a.ResolveMCPAccess(ctx, wraw)
	if err != nil || !access.CanWrite {
		t.Fatal("write key cannot write")
	}
}

func TestMCPKeyPermissionCannotReviveExpiredOrRevokedKeys(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(map[bool]string{false: "revoked", true: "expired"}[expired], func(t *testing.T) {
			a, actor := fixture(t)
			ctx := context.Background()
			now := time.Now().Truncate(time.Second)
			a.Config.Now = func() time.Time { return now }
			key, raw, err := a.CreateMCPTokenWithWrite(ctx, actor, "Writer", 30, true)
			if err != nil {
				t.Fatal(err)
			}
			if expired {
				now = now.Add(30 * 24 * time.Hour)
			} else {
				if err = a.RevokeMCPToken(ctx, actor, key.ID); err != nil {
					t.Fatal(err)
				}
			}
			if err = a.SetMCPTokenWrite(ctx, actor, key.ID, true); !errors.Is(err, domain.ErrNotFound) {
				t.Fatalf("inactive key updated: %v", err)
			}
			if _, err = a.ResolveMCPAccess(ctx, raw); !errors.Is(err, domain.ErrForbidden) {
				t.Fatal("inactive key revived")
			}
		})
	}
}

func TestMCPExistingReadKeySurvivesPermissionMigration(t *testing.T) {
	a, actor := fixture(t)
	ctx := context.Background()
	_, raw, err := a.CreateMCPToken(ctx, actor, "Existing", 90)
	if err != nil {
		t.Fatal(err)
	}
	// Recreate the schema/version state before migration 4, preserving the key.
	if _, err = a.Store.Write.Exec("ALTER TABLE mcp_tokens DROP COLUMN can_write"); err != nil {
		t.Fatal(err)
	}
	if _, err = a.Store.Write.Exec("DELETE FROM goose_db_version WHERE version_id=4"); err != nil {
		t.Fatal(err)
	}
	dir := a.Store.DataDir
	if err = a.Store.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := storage.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	reopened, err := auth.New(s, a.Config)
	if err != nil {
		t.Fatal(err)
	}
	access, err := reopened.ResolveMCPAccess(ctx, raw)
	if err != nil || access.User.ID != actor || access.CanWrite {
		t.Fatalf("existing key migration: %+v %v", access, err)
	}
}
