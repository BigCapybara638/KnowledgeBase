package articles_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/BigCapybara638/KnowledgeBase/internal/articles"
	"github.com/BigCapybara638/KnowledgeBase/internal/content"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/search"
	"github.com/BigCapybara638/KnowledgeBase/internal/testutil"
)

func TestCreateFilledDraftIsPrivateAndPublishable(t *testing.T) {
	s, u, c := seed(t, 0)
	ctx := context.Background()
	service := articles.New(s)
	doc, err := content.PlainDocument("Личный текст 🚀\n\nВторой абзац")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := service.CreateFilledDraft(ctx, u, articles.CreateDraftInput{Title: "  Статья агента  ", CategoryID: c, Document: doc})
	if err != nil {
		t.Fatal(err)
	}
	if draft.OwnerID != u || draft.Title != "Статья агента" || draft.Version != 1 || draft.BaseArticleVersion != 0 {
		t.Fatalf("draft: %+v", draft)
	}
	if _, err = service.Get(ctx, u, draft.ArticleID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("draft is publicly readable")
	}
	other := testutil.User(t, s, "user")
	if _, err = service.GetDraft(ctx, other, draft.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("foreign draft readable")
	}
	found, err := search.New(s).Find(ctx, u, "Личный", 1)
	if err != nil || len(found.Items) != 0 {
		t.Fatal("draft was indexed")
	}
	published, err := service.Publish(ctx, u, draft.ID, draft.Version, draft.BaseArticleVersion)
	if err != nil || published.AuthorID != u || published.PlainText != "Личный текст 🚀\n\nВторой абзац" {
		t.Fatalf("publish: %+v %v", published, err)
	}
}

func TestCreateFilledDraftValidatesBeforeInserting(t *testing.T) {
	s, u, _ := seed(t, 0)
	ctx := context.Background()
	service := articles.New(s)
	doc, err := content.PlainDocument("Текст")
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range []articles.CreateDraftInput{
		{Title: "", Document: doc}, {Title: "\n ", Document: doc}, {Title: strings.Repeat("я", 257), Document: doc},
		{Title: "Name", CategoryID: domain.NewID(), Document: doc}, {Title: "Name", Document: []byte(`{}`)},
	} {
		if _, err := service.CreateFilledDraft(ctx, u, in); err == nil {
			t.Fatal("invalid draft created")
		}
	}
	for _, table := range []string{"articles", "drafts"} {
		var count int
		if err := s.Read.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("orphan in %s: %d %v", table, count, err)
		}
	}
	draft, err := service.CreateFilledDraft(ctx, u, articles.CreateDraftInput{Title: "Без категории", Document: doc})
	if err != nil || draft.CategoryID != "" {
		t.Fatalf("uncategorized draft: %v", err)
	}
}
