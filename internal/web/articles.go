package web

import (
	"github.com/BigCapybara638/KnowledgeBase/internal/categories"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strconv"
)

func pageNumber(r *http.Request) int {
	n, e := strconv.Atoi(r.URL.Query().Get("page"))
	if e != nil || n < 1 {
		return 1
	}
	return n
}
func (a *App) home(w http.ResponseWriter, r *http.Request) {
	var cat *domain.ID
	if id := r.URL.Query().Get("category"); id != "" {
		v := domain.ID(id)
		cat = &v
	}
	p, e := a.articles.List(r.Context(), currentUser(r).ID, cat, pageNumber(r))
	if e != nil {
		a.fail(w, r, e)
		return
	}
	cats, e := a.categories.List(r.Context())
	if e != nil {
		a.fail(w, r, e)
		return
	}
	selected := ""
	if cat != nil {
		selected = string(*cat)
	}
	a.view(w, r, 200, "home.html", map[string]any{"Title": "Все статьи", "Categories": cats, "Articles": p.Items, "Page": p.Page, "NextPage": p.Page + 1, "PrevPage": p.Page - 1, "HasNext": p.HasNext, "SelectedCategory": selected})
}
func (a *App) article(w http.ResponseWriter, r *http.Request) {
	item, e := a.articles.Get(r.Context(), currentUser(r).ID, domain.ID(chi.URLParam(r, "id")))
	if e != nil {
		a.fail(w, r, e)
		return
	}
	cats, e := a.categories.List(r.Context())
	if e != nil {
		a.fail(w, r, e)
		return
	}
	a.view(w, r, 200, "article.html", map[string]any{"Title": item.Title, "Article": item, "Categories": cats, "SelectedCategory": string(item.CategoryID)})
}
func (a *App) categoryMenu(w http.ResponseWriter, r *http.Request) {
	id := domain.ID(chi.URLParam(r, "id"))
	p, e := a.articles.ListCategoryMenu(r.Context(), currentUser(r).ID, id, r.URL.Query().Get("cursor"))
	if e != nil {
		a.fail(w, r, e)
		return
	}
	a.view(w, r, 200, "sidebar_articles.html", map[string]any{"Menu": p, "CategoryID": id})
}
func (a *App) categoriesPage(w http.ResponseWriter, r *http.Request) {
	if e := a.store.Admin(r.Context(), currentUser(r).ID); e != nil {
		a.fail(w, r, e)
		return
	}
	cats, e := a.categories.List(r.Context())
	if e != nil {
		a.fail(w, r, e)
		return
	}
	a.view(w, r, 200, "admin_categories.html", map[string]any{"Title": "Категории", "Categories": cats})
}
func (a *App) saveCategory(w http.ResponseWriter, r *http.Request) {
	order, _ := strconv.Atoi(r.FormValue("order"))
	version, _ := strconv.ParseInt(r.FormValue("version"), 10, 64)
	_, e := a.categories.Save(r.Context(), currentUser(r).ID, categories.Update{ID: domain.ID(chi.URLParam(r, "id")), Title: r.FormValue("title"), Order: order, ExpectedVersion: domain.Version(version)})
	if e != nil {
		a.fail(w, r, e)
		return
	}
	http.Redirect(w, r, "/admin/categories", 303)
}
func (a *App) deleteCategory(w http.ResponseWriter, r *http.Request) {
	version, _ := strconv.ParseInt(r.FormValue("version"), 10, 64)
	if e := a.categories.Delete(r.Context(), currentUser(r).ID, domain.ID(chi.URLParam(r, "id")), domain.Version(version)); e != nil {
		a.fail(w, r, e)
		return
	}
	http.Redirect(w, r, "/admin/categories", 303)
}
