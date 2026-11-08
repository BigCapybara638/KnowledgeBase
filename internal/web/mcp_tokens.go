package web

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/go-chi/chi/v5"
)

func (a *App) renderMCPTokens(w http.ResponseWriter, r *http.Request, status int, raw, message string) {
	keys, err := a.accounts.ListMCPTokens(r.Context(), currentUser(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.view(w, r, status, "mcp_tokens.html", map[string]any{"Title": "Доступ агента", "Tokens": keys, "RawToken": raw, "Error": message, "ServerURL": a.cfg.PublicURL + "/mcp", "Now": time.Now().Unix()})
}

func (a *App) mcpTokensPage(w http.ResponseWriter, r *http.Request) {
	a.renderMCPTokens(w, r, http.StatusOK, "", "")
}

func (a *App) createMCPToken(w http.ResponseWriter, r *http.Request) {
	days := 90
	if value := r.FormValue("days"); value != "" {
		var err error
		days, err = strconv.Atoi(value)
		if err != nil {
			a.renderMCPTokens(w, r, 400, "", "Выберите срок действия ключа.")
			return
		}
	}
	_, raw, err := a.accounts.CreateMCPToken(r.Context(), currentUser(r).ID, r.FormValue("name"), days)
	if errors.Is(err, domain.ErrInvalid) {
		a.renderMCPTokens(w, r, 400, "", "Укажите название от 1 до 80 символов и срок 30, 90 или 365 дней.")
		return
	}
	if errors.Is(err, domain.ErrConflict) {
		a.renderMCPTokens(w, r, 409, "", "Достигнут лимит 20 действующих ключей. Отзовите ненужный ключ.")
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.renderMCPTokens(w, r, http.StatusOK, raw, "")
}

func (a *App) revokeMCPToken(w http.ResponseWriter, r *http.Request) {
	if err := a.accounts.RevokeMCPToken(r.Context(), currentUser(r).ID, domain.ID(chi.URLParam(r, "id"))); err != nil {
		a.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/settings/mcp", http.StatusSeeOther)
}
