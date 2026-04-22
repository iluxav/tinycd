package ui

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/iluxav/tinycd/internal/auth"
)

const (
	cookieName    = "tcd_session"
	ctxUserHeader = "X-Tcd-User" // request-scoped via header trick below
)

// publicPaths are served without auth.
var publicPaths = map[string]struct{}{
	"/login":   {},
	"/logout":  {},
	"/healthz": {},
}

// authMiddleware redirects unauthenticated browser requests to /login,
// and returns 401 for anything else (API-style callers).
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := publicPaths[r.URL.Path]; ok {
			next.ServeHTTP(w, r)
			return
		}
		// Load current auth file on every request so password changes apply
		// immediately without restart. Cheap — auth.yml is small.
		af, err := auth.Load()
		if err != nil {
			if errors.Is(err, auth.ErrNotConfigured) {
				http.Error(w, "auth not configured — run `tcd admin set-password admin` on the host", http.StatusServiceUnavailable)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		c, err := r.Cookie(cookieName)
		if err != nil {
			redirectLogin(w, r)
			return
		}
		user, err := af.ParseCookie(c.Value)
		if err != nil {
			clearSessionCookie(w)
			redirectLogin(w, r)
			return
		}
		// Attach username via a synthetic request header so handlers can read it.
		r.Header.Set(ctxUserHeader, user)
		next.ServeHTTP(w, r)
	})
}

func currentUser(r *http.Request) string {
	return r.Header.Get(ctxUserHeader)
}

func redirectLogin(w http.ResponseWriter, r *http.Request) {
	next := r.URL.Path
	if r.URL.RawQuery != "" {
		next += "?" + r.URL.RawQuery
	}
	target := "/login"
	if next != "" && next != "/login" {
		target += "?next=" + url.QueryEscape(next)
	}
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", target)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func setSessionCookie(w http.ResponseWriter, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		// Secure: true — intentionally omitted so this works over plain HTTP on
		// localhost. If you expose the UI over TLS, set Secure=true above.
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

// loginView is data for login.html.
type loginView struct {
	Next  string
	Error string
}

func (s *Server) handleLoginGET(w http.ResponseWriter, r *http.Request) {
	next := r.URL.Query().Get("next")
	_ = s.tmpl.ExecuteTemplate(w, "login.html", loginView{Next: next})
}

func (s *Server) handleLoginPOST(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	user := strings.TrimSpace(r.FormValue("user"))
	pass := r.FormValue("password")
	next := r.FormValue("next")
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		next = "/"
	}

	af, err := auth.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	if err := af.Verify(user, pass); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_ = s.tmpl.ExecuteTemplate(w, "login.html", loginView{Next: next, Error: "invalid username or password"})
		return
	}
	cookie, err := af.MakeCookie(user, auth.DefaultSessionTTL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	setSessionCookie(w, cookie)
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	clearSessionCookie(w)
	redirectBack(w, r, "/login")
}

// settings page — change password.
func (s *Server) handleSettingsGET(w http.ResponseWriter, r *http.Request) {
	_ = s.tmpl.ExecuteTemplate(w, "settings.html", map[string]any{
		"User": currentUser(r),
	})
}

func (s *Server) handleSettingsPOST(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	user := currentUser(r)
	old := r.FormValue("old_password")
	new1 := r.FormValue("new_password")
	new2 := r.FormValue("new_password_confirm")

	if new1 != new2 {
		renderSettingsError(s, w, r, user, "new passwords do not match")
		return
	}
	af, err := auth.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	if err := af.Verify(user, old); err != nil {
		renderSettingsError(s, w, r, user, "current password is incorrect")
		return
	}
	if err := af.SetPassword(user, new1); err != nil {
		renderSettingsError(s, w, r, user, err.Error())
		return
	}
	if err := af.Save(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.tmpl.ExecuteTemplate(w, "settings.html", map[string]any{
		"User":    user,
		"Success": "password changed",
	})
}

func renderSettingsError(s *Server, w http.ResponseWriter, r *http.Request, user, msg string) {
	w.WriteHeader(http.StatusBadRequest)
	_ = s.tmpl.ExecuteTemplate(w, "settings.html", map[string]any{
		"User":  user,
		"Error": msg,
	})
}
