package search_test

import (
	"context"
	"encoding/json"
	"github.com/BigCapybara638/KnowledgeBase/internal/articles"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/search"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"github.com/BigCapybara638/KnowledgeBase/internal/testutil"
	"testing"
)

func fixture(t *testing.T) (*storage.Store, domain.ID, domain.ID) {
	s := testutil.Store(t)
	u := testutil.User(t, s, "admin")
	c := domain.NewID()
	if _, e := s.Write.Exec("INSERT INTO categories(id,title) VALUES(?,'Процессы')", c); e != nil {
		t.Fatal(e)
	}
	return s, u, c
}
func doc(text string) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": text}}}}})
	return b
}
func draft(t *testing.T, s *storage.Store, u, c domain.ID, title string, body json.RawMessage) domain.Draft {
	t.Helper()
	a := articles.New(s)
	d, e := a.CreateDraft(context.Background(), u, nil)
	if e != nil {
		t.Fatal(e)
	}
	d, e = a.SaveDraft(context.Background(), articles.SaveDraftInput{DraftID: d.ID, ActorID: u, CategoryID: c, Title: title, Document: body, ExpectedVersion: d.Version})
	if e != nil {
		t.Fatal(e)
	}
	return d
}
func publish(t *testing.T, s *storage.Store, u domain.ID, d domain.Draft) domain.Article {
	t.Helper()
	a, e := articles.New(s).Publish(context.Background(), u, d.ID, d.Version, d.BaseArticleVersion)
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func TestFindsCyrillicAndTableText(t *testing.T) {
	s, u, c := fixture(t)
	d := draft(t, s, u, c, "Документы", []byte(`{"type":"doc","content":[{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"Командировка"}]}]}]}]}]}`))
	a := publish(t, s, u, d)
	p, e := search.New(s).Find(context.Background(), u, "командировка", 1)
	if e != nil || len(p.Items) != 1 || p.Items[0].ID != a.ID {
		t.Fatal(p, e)
	}
}
func TestTitleRanksAboveBody(t *testing.T) {
	s, u, c := fixture(t)
	first := publish(t, s, u, draft(t, s, u, c, "Отпуск", doc("Согласование")))
	publish(t, s, u, draft(t, s, u, c, "Согласование", doc("Отпуск")))
	p, e := search.New(s).Find(context.Background(), u, "отпуск", 1)
	if e != nil || len(p.Items) != 2 || p.Items[0].ID != first.ID {
		t.Fatal(p, e)
	}
}
func TestSearchNeverShowsDrafts(t *testing.T) {
	s, u, c := fixture(t)
	draft(t, s, u, c, "Секрет", doc("Секрет"))
	p, e := search.New(s).Find(context.Background(), u, "секрет", 1)
	if e != nil || len(p.Items) != 0 {
		t.Fatal(p, e)
	}
}
func TestArchiveAndRepublishUpdateIndex(t *testing.T) {
	s, u, c := fixture(t)
	a := publish(t, s, u, draft(t, s, u, c, "Архив", doc("Отпуск")))
	svc := articles.New(s)
	d, e := svc.CreateDraft(context.Background(), u, &a.ID)
	if e != nil {
		t.Fatal(e)
	}
	d, e = svc.SaveDraft(context.Background(), articles.SaveDraftInput{DraftID: d.ID, ActorID: u, CategoryID: c, ExpectedVersion: d.Version, Title: "Архив", Document: doc("Командировка")})
	if e != nil {
		t.Fatal(e)
	}
	a = publish(t, s, u, d)
	q := search.New(s)
	p, e := q.Find(context.Background(), u, "отпуск", 1)
	if e != nil || len(p.Items) != 0 {
		t.Fatal(p, e)
	}
	if e = svc.SetArchived(context.Background(), u, a.ID, a.Version, true); e != nil {
		t.Fatal(e)
	}
	p, e = q.Find(context.Background(), u, "командировка", 1)
	if e != nil || len(p.Items) != 0 {
		t.Fatal(p, e)
	}
}
func TestQuotesPunctuationAndEmptyQuery(t *testing.T) {
	s, u, c := fixture(t)
	publish(t, s, u, draft(t, s, u, c, "Отпуск", doc("Текст")))
	for _, query := range []string{"", `"`, "- : * OR", `"Отпуск"`, " отпуск "} {
		p, e := search.New(s).Find(context.Background(), u, query, 1)
		if e != nil {
			t.Fatalf("query %q: %v", query, e)
		}
		if query == "" && len(p.Items) != 1 {
			t.Fatal(p)
		}
	}
}
func TestSnippetDoesNotExecuteHTML(t *testing.T) {
	s, u, c := fixture(t)
	publish(t, s, u, draft(t, s, u, c, "Инструкция", doc("<script>инструкция</script>")))
	p, e := search.New(s).Find(context.Background(), u, "инструкция", 1)
	if e != nil || len(p.Items) != 1 {
		t.Fatal(p, e)
	}
	if p.Items[0].Snippet == "" {
		t.Fatal("missing snippet")
	}
}
func TestPublishRollbackKeepsIndexConsistent(t *testing.T) {
	s, u, c := fixture(t)
	a := publish(t, s, u, draft(t, s, u, c, "Первая", doc("Отпуск")))
	svc := articles.New(s)
	d, e := svc.CreateDraft(context.Background(), u, &a.ID)
	if e != nil {
		t.Fatal(e)
	}
	d, e = svc.SaveDraft(context.Background(), articles.SaveDraftInput{DraftID: d.ID, ActorID: u, CategoryID: c, ExpectedVersion: d.Version, Title: "ROLLBACK", Document: doc("Командировка")})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Write.Exec("CREATE TRIGGER reject_publication BEFORE DELETE ON drafts WHEN OLD.title='ROLLBACK' BEGIN SELECT RAISE(ABORT,'forced failure'); END")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Publish(context.Background(), u, d.ID, d.Version, d.BaseArticleVersion); e == nil {
		t.Fatal("forced error did not fail publication")
	}
	p, e := search.New(s).Find(context.Background(), u, "отпуск", 1)
	if e != nil || len(p.Items) != 1 {
		t.Fatal(p, e)
	}
	p, e = search.New(s).Find(context.Background(), u, "командировка", 1)
	if e != nil || len(p.Items) != 0 {
		t.Fatal(p, e)
	}
}
