package articles_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/BigCapybara638/KnowledgeBase/internal/articles"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"github.com/BigCapybara638/KnowledgeBase/internal/testutil"
	"sync"
	"testing"
)

func document(text string) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": text}}}}})
	return b
}
func edit(t *testing.T, s *storage.Store, actor, category domain.ID, id *domain.ID, title, text string) domain.Draft {
	t.Helper()
	svc := articles.New(s)
	d, e := svc.CreateDraft(context.Background(), actor, id)
	if e != nil {
		t.Fatal(e)
	}
	d, e = svc.SaveDraft(context.Background(), articles.SaveDraftInput{DraftID: d.ID, ActorID: actor, CategoryID: category, ExpectedVersion: d.Version, Title: title, Document: document(text)})
	if e != nil {
		t.Fatal(e)
	}
	return d
}
func TestOnlyOwnerReadsDraft(t *testing.T) {
	s, u, c := seed(t, 0)
	d := edit(t, s, u, c, nil, "Черновик", "Секрет")
	other := testutil.User(t, s, "user")
	if _, e := articles.New(s).GetDraft(context.Background(), other, d.ID); !errors.Is(e, domain.ErrNotFound) {
		t.Fatalf("private draft leaked: %v", e)
	}
}
func TestPublishedTextStaysUntilRepublish(t *testing.T) {
	s, u, c := seed(t, 1)
	id := domain.ID("article-000")
	d := edit(t, s, u, c, &id, "Правка", "Новый текст")
	a := articles.New(s)
	old, e := a.Get(context.Background(), u, id)
	if e != nil || old.Title != "Статья 000" {
		t.Fatal(old, e)
	}
	p, e := a.Publish(context.Background(), u, d.ID, d.Version, d.BaseArticleVersion)
	if e != nil || p.PlainText != "Новый текст" || p.Version != 2 {
		t.Fatal(p, e)
	}
}
func TestConcurrentPublishExactlyOneWinner(t *testing.T) {
	s, _, c := seed(t, 1)
	a := articles.New(s)
	id := domain.ID("article-000")
	ds := []domain.Draft{}
	for i := range 5 {
		u := testutil.User(t, s, "user")
		ds = append(ds, edit(t, s, u, c, &id, fmt.Sprintf("Публикация %d", i), fmt.Sprintf("Текст %d", i)))
	}
	start := make(chan struct{})
	out := make(chan error, 5)
	var wg sync.WaitGroup
	for _, d := range ds {
		wg.Go(func() {
			<-start
			_, e := a.Publish(context.Background(), d.OwnerID, d.ID, d.Version, d.BaseArticleVersion)
			out <- e
		})
	}
	close(start)
	wg.Wait()
	close(out)
	ok, conflicts := 0, 0
	for e := range out {
		if e == nil {
			ok++
		} else if errors.Is(e, domain.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if ok != 1 || conflicts != 4 {
		t.Fatalf("successes=%d conflicts=%d", ok, conflicts)
	}
	var v, n int
	s.Read.QueryRow("SELECT version FROM articles WHERE id=?", id).Scan(&v)
	s.Read.QueryRow("SELECT count(*) FROM articles_fts WHERE article_id=?", id).Scan(&n)
	if v != 2 || n != 1 {
		t.Fatalf("inconsistent version/index %d/%d", v, n)
	}
}
func TestStaleRetryStillConflicts(t *testing.T) {
	s, u, c := seed(t, 1)
	id := domain.ID("article-000")
	d := edit(t, s, u, c, &id, "Правка", "Текст")
	a := articles.New(s)
	if _, e := a.Publish(context.Background(), u, d.ID, d.Version, d.BaseArticleVersion); e != nil {
		t.Fatal(e)
	}
	if _, e := a.Publish(context.Background(), u, d.ID, d.Version, d.BaseArticleVersion); !errors.Is(e, domain.ErrConflict) {
		t.Fatalf("stale retry accepted: %v", e)
	}
}
func TestSameUserTwoTabsConflict(t *testing.T) {
	s, u, c := seed(t, 0)
	d := edit(t, s, u, c, nil, "Текст", "Первая вкладка")
	a := articles.New(s)
	in := articles.SaveDraftInput{DraftID: d.ID, ActorID: u, CategoryID: c, ExpectedVersion: d.Version, Title: "Правка", Document: document("Одна вкладка")}
	if _, e := a.SaveDraft(context.Background(), in); e != nil {
		t.Fatal(e)
	}
	in.Document = document("Другая вкладка")
	if _, e := a.SaveDraft(context.Background(), in); !errors.Is(e, domain.ErrConflict) {
		t.Fatal(e)
	}
	got, _ := a.GetDraft(context.Background(), u, d.ID)
	if string(got.Document) != string(document("Одна вкладка")) {
		t.Fatal("successful text overwritten")
	}
}
func TestOriginalAuthorAndLastEditorDiffer(t *testing.T) {
	s, u, c := seed(t, 1)
	other := testutil.User(t, s, "user")
	id := domain.ID("article-000")
	d := edit(t, s, other, c, &id, "Правка", "Текст")
	p, e := articles.New(s).Publish(context.Background(), other, d.ID, d.Version, d.BaseArticleVersion)
	if e != nil || p.AuthorID != u || p.LastEditorID != other {
		t.Fatal(p, e)
	}
}
func TestArchiveCanBeRestoredByAdmin(t *testing.T) {
	s, u, _ := seed(t, 1)
	admin := testutil.User(t, s, "admin")
	a := articles.New(s)
	id := domain.ID("article-000")
	if e := a.SetArchived(context.Background(), u, id, 1, true); !errors.Is(e, domain.ErrForbidden) {
		t.Fatal(e)
	}
	if e := a.SetArchived(context.Background(), admin, id, 1, true); e != nil {
		t.Fatal(e)
	}
	if _, e := a.Get(context.Background(), u, id); !errors.Is(e, domain.ErrNotFound) {
		t.Fatal(e)
	}
	if e := a.SetArchived(context.Background(), admin, id, 2, false); e != nil {
		t.Fatal(e)
	}
	if _, e := a.Get(context.Background(), u, id); e != nil {
		t.Fatal(e)
	}
}
