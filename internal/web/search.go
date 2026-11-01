package web

import "net/http"

func (a *App) searchPage(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	p, e := a.search.Find(r.Context(), currentUser(r).ID, query, pageNumber(r))
	if e != nil {
		a.fail(w, r, e)
		return
	}
	cats, e := a.categories.List(r.Context())
	if e != nil {
		a.fail(w, r, e)
		return
	}
	a.view(w, r, 200, "search.html", map[string]any{"Title": "Поиск", "Query": query, "Results": p.Items, "Categories": cats, "Page": p.Page, "NextPage": p.Page + 1, "PrevPage": p.Page - 1, "HasNext": p.HasNext})
}
