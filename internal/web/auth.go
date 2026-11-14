package web

import (
	"github.com/BigCapybara638/KnowledgeBase/internal/auth"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strconv"
	"time"
)

func (a *App) loginPage(w http.ResponseWriter, r *http.Request) {
	a.view(w, r, 200, "login.html", map[string]any{"Title": "Вход", "ReturnTo": safeReturnTo(r.URL.Query().Get("return_to"))})
}
func (a *App) setSession(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{Name: "kb_session", Value: token, Path: "/", HttpOnly: true, Secure: !a.cfg.Dev, SameSite: http.SameSiteLaxMode, MaxAge: int(auth.SessionLifetime / time.Second), Expires: time.Now().Add(auth.SessionLifetime)})
}
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	l, e := a.accounts.Login(r.Context(), r.FormValue("login"), r.FormValue("password"))
	if e != nil {
		a.view(w, r, 401, "login.html", map[string]any{"Title": "Вход", "Error": "Неверный логин или пароль, либо вход временно ограничен.", "ReturnTo": safeReturnTo(r.FormValue("return_to"))})
		return
	}
	a.setSession(w, l.RawToken)
	target := safeReturnTo(r.FormValue("return_to"))
	if l.MustChangePassword {
		target = "/password"
	}
	http.Redirect(w, r, target, 303)
}
func (a *App) passwordPage(w http.ResponseWriter, r *http.Request) {
	a.view(w, r, 200, "password.html", map[string]any{"Title": "Смена пароля"})
}
func (a *App) password(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	newPassword := r.FormValue("new_password")
	if e := a.accounts.ChangePassword(r.Context(), u.ID, r.FormValue("old_password"), newPassword); e != nil {
		a.view(w, r, 400, "password.html", map[string]any{"Title": "Смена пароля", "Error": "Проверьте текущий пароль и длину нового пароля."})
		return
	}
	l, e := a.accounts.Login(r.Context(), u.Login, newPassword)
	if e != nil {
		http.Redirect(w, r, "/login", 303)
		return
	}
	a.setSession(w, l.RawToken)
	http.Redirect(w, r, "/", 303)
}
func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if c, e := r.Cookie("kb_session"); e == nil {
		if e = a.accounts.Logout(r.Context(), c.Value); e != nil {
			a.fail(w, r, e)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{Name: "kb_session", Value: "", Path: "/", HttpOnly: true, Secure: !a.cfg.Dev, MaxAge: -1})
	http.Redirect(w, r, "/login", 303)
}
func (a *App) usersPage(w http.ResponseWriter, r *http.Request) {
	users, e := a.accounts.Users(r.Context(), currentUser(r).ID)
	if e != nil {
		a.fail(w, r, e)
		return
	}
	a.view(w, r, 200, "admin_users.html", map[string]any{"Title": "Сотрудники", "Users": users})
}
func (a *App) createUser(w http.ResponseWriter, r *http.Request) {
	_, e := a.accounts.CreateUser(r.Context(), currentUser(r).ID, auth.NewUser{Login: r.FormValue("login"), Name: r.FormValue("name"), TemporaryPassword: r.FormValue("password"), Role: r.FormValue("role")})
	if e != nil {
		a.fail(w, r, e)
		return
	}
	http.Redirect(w, r, "/admin/users", 303)
}
func (a *App) resetPassword(w http.ResponseWriter, r *http.Request) {
	e := a.accounts.ResetPassword(r.Context(), currentUser(r).ID, domain.ID(chi.URLParam(r, "id")), r.FormValue("password"))
	if e != nil {
		a.fail(w, r, e)
		return
	}
	http.Redirect(w, r, "/admin/users", 303)
}
func (a *App) disableUser(w http.ResponseWriter, r *http.Request) {
	disabled, e := strconv.ParseBool(r.FormValue("disabled"))
	if e != nil {
		a.fail(w, r, domain.ErrInvalid)
		return
	}
	e = a.accounts.SetDisabled(r.Context(), currentUser(r).ID, domain.ID(chi.URLParam(r, "id")), disabled)
	if e != nil {
		a.fail(w, r, e)
		return
	}
	http.Redirect(w, r, "/admin/users", 303)
}
