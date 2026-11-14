package articles_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/BigCapybara638/KnowledgeBase/internal/articles"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"github.com/BigCapybara638/KnowledgeBase/internal/testutil"
)

func assertDeleteCount(t *testing.T, s *storage.Store, want int, query string, args ...any) {
	t.Helper()
	var got int
	if e := s.Read.QueryRow(query, args...).Scan(&got); e != nil {
		t.Fatal(e)
	}
	if got != want {
		t.Fatalf("%s: got %d, want %d", query, got, want)
	}
}

func TestDeleteDraftRemovesUnpublishedPlaceholder(t *testing.T) {
	s, owner, category := seed(t, 0)
	d := edit(t, s, owner, category, nil, "Private", "Unpublished text")
	a := articles.New(s)
	if e := a.DeleteDraft(context.Background(), owner, d.ID, d.Version); e != nil {
		t.Fatal(e)
	}
	if _, e := a.GetDraft(context.Background(), owner, d.ID); !errors.Is(e, domain.ErrNotFound) {
		t.Fatalf("deleted draft remains readable: %v", e)
	}
	drafts, e := a.Drafts(context.Background(), owner)
	if e != nil || len(drafts) != 0 {
		t.Fatalf("deleted draft remains in list: %+v %v", drafts, e)
	}
	assertDeleteCount(t, s, 0, "SELECT count(*) FROM articles WHERE id=?", d.ArticleID)
}

func TestDeleteDraftPreservesPublishedArticleAndSiblingDrafts(t *testing.T) {
	s, owner, category := seed(t, 1)
	a := articles.New(s)
	articleID := domain.ID("article-000")
	published, e := a.Get(context.Background(), owner, articleID)
	if e != nil {
		t.Fatal(e)
	}
	target := edit(t, s, owner, category, &articleID, "Discard", "Private changes")
	ownerSibling := edit(t, s, owner, category, &articleID, "Keep mine", "Another private draft")
	other := testutil.User(t, s, "user")
	otherSibling := edit(t, s, other, category, &articleID, "Keep theirs", "Someone else's draft")
	if e := a.DeleteDraft(context.Background(), owner, target.ID, target.Version); e != nil {
		t.Fatal(e)
	}
	got, e := a.Get(context.Background(), owner, articleID)
	if e != nil || !reflect.DeepEqual(got, published) {
		t.Fatalf("published article changed: got %+v, want %+v, error %v", got, published, e)
	}
	for _, want := range []domain.Draft{ownerSibling, otherSibling} {
		got, e := a.GetDraft(context.Background(), want.OwnerID, want.ID)
		if e != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("sibling draft changed: got %+v, want %+v, error %v", got, want, e)
		}
	}
	assertDeleteCount(t, s, 0, "SELECT count(*) FROM drafts WHERE id=?", target.ID)
}

func TestDeleteDraftKeepsUnpublishedPlaceholderWhileSiblingExists(t *testing.T) {
	s, owner, category := seed(t, 0)
	target := edit(t, s, owner, category, nil, "Discard", "Private changes")
	siblingID := domain.NewID()
	if _, e := s.Write.Exec("INSERT INTO drafts(id,article_id,owner_id,document,base_article_version,updated_at) VALUES(?,?,?,?,0,1)", siblingID, target.ArticleID, owner, document("Keep")); e != nil {
		t.Fatal(e)
	}
	a := articles.New(s)
	if e := a.DeleteDraft(context.Background(), owner, target.ID, target.Version); e != nil {
		t.Fatal(e)
	}
	assertDeleteCount(t, s, 1, "SELECT count(*) FROM articles WHERE id=?", target.ArticleID)
	if _, e := a.GetDraft(context.Background(), owner, siblingID); e != nil {
		t.Fatalf("unpublished sibling removed: %v", e)
	}
	if e := a.DeleteDraft(context.Background(), owner, siblingID, 1); e != nil {
		t.Fatal(e)
	}
	assertDeleteCount(t, s, 0, "SELECT count(*) FROM articles WHERE id=?", target.ArticleID)
}

func TestDeleteDraftPreservesArchivedArticle(t *testing.T) {
	s, owner, category := seed(t, 1)
	articleID := domain.ID("article-000")
	d := edit(t, s, owner, category, &articleID, "Discard", "Private changes")
	admin := testutil.User(t, s, "admin")
	a := articles.New(s)
	if e := a.SetArchived(context.Background(), admin, articleID, 1, true); e != nil {
		t.Fatal(e)
	}
	if e := a.DeleteDraft(context.Background(), owner, d.ID, d.Version); e != nil {
		t.Fatal(e)
	}
	archived, e := a.Archived(context.Background(), admin)
	if e != nil || len(archived) != 1 || archived[0].ID != articleID || archived[0].Version != 2 || archived[0].Title != "Статья 000" {
		t.Fatalf("archived publication changed: %+v %v", archived, e)
	}
}

func TestDeleteDraftRequiresOwnerAndCurrentVersion(t *testing.T) {
	tests := []struct {
		name          string
		role          string
		useOwner      bool
		missingDraft  bool
		versionOffset domain.Version
		want          error
	}{
		{name: "other user", role: "user", want: domain.ErrNotFound},
		{name: "administrator", role: "admin", want: domain.ErrNotFound},
		{name: "nonowner stale version", role: "user", versionOffset: -1, want: domain.ErrNotFound},
		{name: "missing draft", useOwner: true, missingDraft: true, want: domain.ErrNotFound},
		{name: "stale version", useOwner: true, versionOffset: -1, want: domain.ErrConflict},
		{name: "future version", useOwner: true, versionOffset: 1, want: domain.ErrConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, owner, category := seed(t, 0)
			d := edit(t, s, owner, category, nil, "Keep", "Private text")
			actor := owner
			if !tt.useOwner {
				actor = testutil.User(t, s, tt.role)
			}
			id := d.ID
			if tt.missingDraft {
				id = domain.NewID()
			}
			a := articles.New(s)
			if e := a.DeleteDraft(context.Background(), actor, id, d.Version+tt.versionOffset); !errors.Is(e, tt.want) {
				t.Fatalf("got %v, want %v", e, tt.want)
			}
			got, e := a.GetDraft(context.Background(), owner, d.ID)
			if e != nil || !reflect.DeepEqual(got, d) {
				t.Fatalf("rejected deletion changed draft: %+v %v", got, e)
			}
			assertDeleteCount(t, s, 1, "SELECT count(*) FROM articles WHERE id=?", d.ArticleID)
		})
	}
}

func TestDeleteDraftRechecksActorAfterWaitingForWriter(t *testing.T) {
	for _, restriction := range []string{"disabled", "must_change"} {
		t.Run(restriction, func(t *testing.T) {
			s, owner, category := seed(t, 0)
			d := edit(t, s, owner, category, nil, "Keep", "Private text")
			tx, e := s.Write.BeginTx(context.Background(), nil)
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback()
			before := s.Write.Stats().WaitCount
			done := make(chan error, 1)
			go func() {
				done <- articles.New(s).DeleteDraft(context.Background(), owner, d.ID, d.Version)
			}()
			await(t, func() bool { return s.Write.Stats().WaitCount > before })
			if _, e = tx.Exec("UPDATE users SET "+restriction+"=1 WHERE id=?", owner); e != nil {
				t.Fatal(e)
			}
			if e = tx.Commit(); e != nil {
				t.Fatal(e)
			}
			if e = <-done; !errors.Is(e, domain.ErrForbidden) {
				t.Fatalf("restricted owner deleted draft: %v", e)
			}
			assertDeleteCount(t, s, 1, "SELECT count(*) FROM drafts WHERE id=? AND version=?", d.ID, d.Version)
			assertDeleteCount(t, s, 1, "SELECT count(*) FROM articles WHERE id=?", d.ArticleID)
		})
	}
}

func TestDeleteDraftRemovesOnlyItsMediaReferences(t *testing.T) {
	s, owner, category := seed(t, 1)
	articleID := domain.ID("article-000")
	target := edit(t, s, owner, category, &articleID, "Discard", "Private changes")
	sibling := edit(t, s, owner, category, &articleID, "Keep", "Other changes")
	shared, private := domain.NewID(), domain.NewID()
	for _, id := range []domain.ID{shared, private} {
		if _, e := s.Write.Exec("INSERT INTO media(id,owner_id,mime,size,width,height,created_at) VALUES(?,?,'image/png',10,1,1,1)", id, owner); e != nil {
			t.Fatal(e)
		}
		if _, e := s.Write.Exec("INSERT INTO draft_media(draft_id,media_id) VALUES(?,?)", target.ID, id); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := s.Write.Exec("INSERT INTO draft_media(draft_id,media_id) VALUES(?,?)", sibling.ID, shared); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Write.Exec("INSERT INTO article_media(article_id,media_id) VALUES(?,?)", articleID, shared); e != nil {
		t.Fatal(e)
	}
	a := articles.New(s)
	if e := a.DeleteDraft(context.Background(), owner, target.ID, target.Version-1); !errors.Is(e, domain.ErrConflict) {
		t.Fatalf("stale media deletion: %v", e)
	}
	assertDeleteCount(t, s, 2, "SELECT count(*) FROM draft_media WHERE draft_id=?", target.ID)
	if e := a.DeleteDraft(context.Background(), owner, target.ID, target.Version); e != nil {
		t.Fatal(e)
	}
	assertDeleteCount(t, s, 0, "SELECT count(*) FROM draft_media WHERE draft_id=?", target.ID)
	assertDeleteCount(t, s, 1, "SELECT count(*) FROM draft_media WHERE draft_id=? AND media_id=?", sibling.ID, shared)
	assertDeleteCount(t, s, 1, "SELECT count(*) FROM article_media WHERE article_id=? AND media_id=?", articleID, shared)
	assertDeleteCount(t, s, 2, "SELECT count(*) FROM media WHERE id IN (?,?)", shared, private)
}
