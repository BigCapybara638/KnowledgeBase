package web

import (
	"encoding/json"
	"github.com/BigCapybara638/KnowledgeBase/internal/articles"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/go-chi/chi/v5"
	"io"
	"net/http"
	"strconv"
)

func (a *App) draftsPage(w http.ResponseWriter, r *http.Request) {
	ds, e := a.articles.Drafts(r.Context(), currentUser(r).ID)
	if e != nil {
		a.fail(w, r, e)
		return
	}
	a.view(w, r, 200, "drafts.html", map[string]any{"Title": "Мои черновики", "Drafts": ds})
}
func (a *App) createDraft(w http.ResponseWriter, r *http.Request) {
	var articleID *domain.ID
	if raw := chi.URLParam(r, "id"); raw != "" {
		id := domain.ID(raw)
		articleID = &id
	}
	d, e := a.articles.CreateDraft(r.Context(), currentUser(r).ID, articleID)
	if e != nil {
		a.fail(w, r, e)
		return
	}
	http.Redirect(w, r, "/drafts/"+string(d.ID)+"/edit", 303)
}
func (a *App) editor(w http.ResponseWriter, r *http.Request) {
	d, e := a.articles.GetDraft(r.Context(), currentUser(r).ID, domain.ID(chi.URLParam(r, "id")))
	if e != nil {
		a.fail(w, r, e)
		return
	}
	cats, e := a.categories.List(r.Context())
	if e != nil {
		a.fail(w, r, e)
		return
	}
	state := map[string]any{"id": d.ID, "articleId": d.ArticleID, "version": d.Version, "baseVersion": d.BaseArticleVersion, "document": d.Document}
	a.view(w, r, 200, "editor.html", map[string]any{"Title": "Редактор статьи", "Draft": d, "Categories": cats, "EditorState": state})
}
func readJSON(w http.ResponseWriter, r *http.Request, out any) error {
	r.Body = http.MaxBytesReader(w, r.Body, (2<<20)+(64<<10))
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return domain.ErrInvalid
	}
	if d.Decode(new(any)) != io.EOF {
		return domain.ErrInvalid
	}
	return nil
}
func jsonReply(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(value)
}
func (a *App) saveDraft(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title           string          `json:"title"`
		CategoryID      domain.ID       `json:"categoryId"`
		ExpectedVersion domain.Version  `json:"expectedVersion"`
		Document        json.RawMessage `json:"document"`
	}
	if e := readJSON(w, r, &body); e != nil {
		a.fail(w, r, e)
		return
	}
	d, e := a.articles.SaveDraft(r.Context(), articles.SaveDraftInput{DraftID: domain.ID(chi.URLParam(r, "id")), ActorID: currentUser(r).ID, CategoryID: body.CategoryID, ExpectedVersion: body.ExpectedVersion, Title: body.Title, Document: body.Document})
	if e != nil {
		a.fail(w, r, e)
		return
	}
	jsonReply(w, map[string]any{"id": d.ID, "version": d.Version, "baseVersion": d.BaseArticleVersion})
}
func (a *App) publishDraft(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ExpectedDraftVersion   domain.Version `json:"expectedDraftVersion"`
		ExpectedArticleVersion domain.Version `json:"expectedArticleVersion"`
	}
	if e := readJSON(w, r, &body); e != nil {
		a.fail(w, r, e)
		return
	}
	item, e := a.articles.Publish(r.Context(), currentUser(r).ID, domain.ID(chi.URLParam(r, "id")), body.ExpectedDraftVersion, body.ExpectedArticleVersion)
	if e != nil {
		a.fail(w, r, e)
		return
	}
	jsonReply(w, map[string]any{"id": item.ID, "version": item.Version, "url": "/articles/" + string(item.ID)})
}
func (a *App) archiveArticle(w http.ResponseWriter, r *http.Request) {
	version, _ := strconv.ParseInt(r.FormValue("version"), 10, 64)
	archived, e := strconv.ParseBool(r.FormValue("archived"))
	if e != nil {
		a.fail(w, r, domain.ErrInvalid)
		return
	}
	id := domain.ID(chi.URLParam(r, "id"))
	if e = a.articles.SetArchived(r.Context(), currentUser(r).ID, id, domain.Version(version), archived); e != nil {
		a.fail(w, r, e)
		return
	}
	http.Redirect(w, r, "/admin/archive", 303)
}
func (a *App) archivePage(w http.ResponseWriter, r *http.Request) {
	rows, e := a.articles.Archived(r.Context(), currentUser(r).ID)
	if e != nil {
		a.fail(w, r, e)
		return
	}
	a.view(w, r, 200, "archive.html", map[string]any{"Title": "Архив статей", "Articles": rows})
}
