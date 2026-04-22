package ui

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"

	"github.com/iluxav/tinycd/internal/config"
)

//go:embed templates/*.html
var templatesFS embed.FS

// Server holds parsed templates and request handlers. One instance per tcd ui
// process. Routes are registered via Handler().
//
// Each page is parsed into its own template set alongside base.html. Go's
// html/template uses a single namespace per set — if multiple pages defined
// {{define "content"}} within the same set, the last parse would win.
type Server struct {
	pages map[string]*template.Template // page name ("index.html") → tree rooted at "base"
}

func New() (*Server, error) {
	pages := map[string]*template.Template{}
	// Pages that extend base.html via block overrides.
	for _, name := range []string{"index.html", "app.html", "settings.html"} {
		t, err := template.New(name).Funcs(funcMap()).ParseFS(templatesFS, "templates/base.html", "templates/"+name)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		pages[name] = t
	}
	// Standalone pages.
	login, err := template.New("login.html").ParseFS(templatesFS, "templates/login.html")
	if err != nil {
		return nil, fmt.Errorf("parse login.html: %w", err)
	}
	pages["login.html"] = login
	return &Server{pages: pages}, nil
}

// render looks up the page template and executes it with the given data.
// For pages that extend base.html, it executes the "base" template. For
// standalone pages (login), it executes the page by name.
func (s *Server) render(w http.ResponseWriter, page string, data any) {
	t, ok := s.pages[page]
	if !ok {
		http.Error(w, "template not found: "+page, http.StatusInternalServerError)
		return
	}
	entry := "base"
	if page == "login.html" {
		entry = "login.html"
	}
	if err := t.ExecuteTemplate(w, entry, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Handler returns the HTTP handler with all routes wired.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Public routes (auth middleware short-circuits these).
	mux.HandleFunc("GET /login", s.handleLoginGET)
	mux.HandleFunc("POST /login", s.handleLoginPOST)
	mux.HandleFunc("POST /logout", s.handleLogout)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	// Protected routes.
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /apps/{name}", s.handleAppDetail)
	mux.HandleFunc("GET /apps/{name}/logs", s.handleLogs)
	mux.HandleFunc("POST /apps/{name}/restart", s.handleRestart)
	mux.HandleFunc("POST /apps/{name}/stop", s.handleStop)
	mux.HandleFunc("POST /apps/{name}/rm", s.handleRm)
	mux.HandleFunc("POST /apps/{name}/env", s.handleEnvUpload)
	mux.HandleFunc("POST /apps/{name}/redeploy", s.handleRedeploy)
	mux.HandleFunc("POST /apps/{name}/scale", s.handleScale)
	mux.HandleFunc("POST /deploy", s.handleDeploy)
	mux.HandleFunc("GET /settings", s.handleSettingsGET)
	mux.HandleFunc("POST /settings", s.handleSettingsPOST)

	return logMiddleware(s.authMiddleware(mux))
}

func (s *Server) loadCfg(w http.ResponseWriter) (*config.Config, bool) {
	cfg, err := config.Load()
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w,
			`<!doctype html><body style="font-family:sans-serif;max-width:600px;margin:64px auto;padding:0 24px"><h2>tcd is not initialized</h2><p>Run on the server:</p><pre style="background:#f0f0f0;padding:12px;border-radius:6px">tcd init --domain your-domain.com</pre><p style="color:#666">%s</p></body>`,
			err.Error(),
		)
		return nil, false
	}
	return cfg, true
}

func funcMap() template.FuncMap {
	return template.FuncMap{
		"short": func(s string) string {
			if len(s) > 7 {
				return s[:7]
			}
			return s
		},
	}
}
