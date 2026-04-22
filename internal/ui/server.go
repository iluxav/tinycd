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
type Server struct {
	tmpl *template.Template
}

func New() (*Server, error) {
	tmpl, err := template.New("").Funcs(funcMap()).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	return &Server{tmpl: tmpl}, nil
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
