package ui

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/iluxav/tinycd/internal/compose"
	"github.com/iluxav/tinycd/internal/config"
	"github.com/iluxav/tinycd/internal/dc"
	"github.com/iluxav/tinycd/internal/deploy"
)

// indexView is the data passed to index.html.
type indexView struct {
	User string
	Cfg  *config.Config
	Apps []*config.AppState
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.loadCfg(w)
	if !ok {
		return
	}
	apps, err := listApps(cfg)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.render(w, "index.html", indexView{
		User: currentUser(r),
		Cfg:  cfg,
		Apps: apps,
	})
}

type appView struct {
	User   string
	Cfg    *config.Config
	App    *config.AppState
	PsText string
	EnvRaw string
}

func (s *Server) handleAppDetail(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.loadCfg(w)
	if !ok {
		return
	}
	name := r.PathValue("name")
	state, err := config.LoadState(cfg.AppDir(name))
	if err != nil {
		http.Error(w, "app not found: "+name, 404)
		return
	}
	client := &dc.Client{RootFile: cfg.RootComposeFile(), Project: "tcd", Env: []string{"ACME_EMAIL=" + cfg.ACMEEmail}}
	psText, _ := capturePs(client, state.Service)
	envRaw, _ := os.ReadFile(state.EnvFile)

	s.render(w, "app.html", appView{
		User:   currentUser(r),
		Cfg:    cfg,
		App:    state,
		PsText: psText,
		EnvRaw: string(envRaw),
	})
}

// handleLogs streams `docker compose logs -f <service>` as Server-Sent Events.
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.loadCfg(w)
	if !ok {
		return
	}
	name := r.PathValue("name")
	state, err := config.LoadState(cfg.AppDir(name))
	if err != nil {
		http.Error(w, "app not found", 404)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", 500)
		return
	}

	tail := r.URL.Query().Get("tail")
	if tail == "" {
		tail = "200"
	}
	args := []string{"compose", "-f", cfg.RootComposeFile(), "-p", "tcd", "logs", "--tail", tail, "-f", "--no-color", state.Service}
	cmd := exec.CommandContext(r.Context(), "docker", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer cmd.Process.Kill()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		select {
		case <-r.Context().Done():
			return
		default:
		}
		line := sseEscape(scanner.Text())
		fmt.Fprintf(w, "data: %s\n\n", line)
		flusher.Flush()
	}
}

func sseEscape(s string) string {
	// SSE: each data line must not contain raw newlines. Our scanner splits on
	// lines already, so just strip carriage returns from DOS-flavored output.
	return strings.ReplaceAll(s, "\r", "")
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	s.doLifecycle(w, r, "restart")
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	s.doLifecycle(w, r, "stop")
}

func (s *Server) doLifecycle(w http.ResponseWriter, r *http.Request, op string) {
	cfg, ok := s.loadCfg(w)
	if !ok {
		return
	}
	name := r.PathValue("name")
	state, err := config.LoadState(cfg.AppDir(name))
	if err != nil {
		http.Error(w, "app not found", 404)
		return
	}
	client := &dc.Client{RootFile: cfg.RootComposeFile(), Project: "tcd", Env: []string{"ACME_EMAIL=" + cfg.ACMEEmail}}
	switch op {
	case "restart":
		err = client.Restart(state.Service)
	case "stop":
		err = client.Stop(state.Service)
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	redirectBack(w, r, "/apps/"+name)
}

func (s *Server) handleRm(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.loadCfg(w)
	if !ok {
		return
	}
	name := r.PathValue("name")
	purge := r.FormValue("purge") == "true"
	if err := deploy.Remove(cfg, name, purge); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	redirectBack(w, r, "/")
}

func (s *Server) handleEnvUpload(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.loadCfg(w)
	if !ok {
		return
	}
	name := r.PathValue("name")
	state, err := config.LoadState(cfg.AppDir(name))
	if err != nil {
		http.Error(w, "app not found", 404)
		return
	}
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	// Two modes: file upload, or raw textarea content.
	var data []byte
	if file, _, err := r.FormFile("envfile"); err == nil {
		defer file.Close()
		data, _ = io.ReadAll(file)
	} else if raw := r.FormValue("envraw"); raw != "" {
		data = []byte(raw)
	} else {
		http.Error(w, "no env content supplied", 400)
		return
	}
	if err := os.MkdirAll(filepath.Dir(state.EnvFile), 0o755); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := os.WriteFile(state.EnvFile, data, 0o600); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	redirectBack(w, r, "/apps/"+name)
}

func (s *Server) handleDeploy(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.loadCfg(w)
	if !ok {
		return
	}
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		// fall back to normal form parse
		_ = r.ParseForm()
	}
	opts := deploy.Options{
		Repo:    strings.TrimSpace(r.FormValue("repo")),
		Name:    strings.TrimSpace(r.FormValue("name")),
		Ref:     strings.TrimSpace(r.FormValue("ref")),
		Service: strings.TrimSpace(r.FormValue("service")),
	}
	if p := r.FormValue("port"); p != "" {
		opts.Port, _ = strconv.Atoi(p)
	}
	if sc := r.FormValue("scale"); sc != "" {
		opts.Scale, _ = strconv.Atoi(sc)
	}
	if a := strings.TrimSpace(r.FormValue("aliases")); a != "" {
		for _, line := range strings.Split(a, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				opts.Aliases = append(opts.Aliases, line)
			}
		}
	}
	// Optional env file upload → write to temp path and pass as EnvFile.
	if file, _, err := r.FormFile("envfile"); err == nil {
		defer file.Close()
		tmp, err := os.CreateTemp("", "tcd-env-*")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		io.Copy(tmp, file)
		tmp.Close()
		defer os.Remove(tmp.Name())
		opts.EnvFile = tmp.Name()
	}

	state, err := deploy.Deploy(cfg, opts)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	redirectBack(w, r, "/apps/"+state.Name)
}

// ---------- helpers ----------

// listApps gathers every app known to the root compose's include list.
func listApps(cfg *config.Config) ([]*config.AppState, error) {
	names, err := compose.ListIncludes(cfg.RootComposeFile())
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	out := make([]*config.AppState, 0, len(names))
	for _, n := range names {
		s, err := config.LoadState(cfg.AppDir(n))
		if err != nil {
			s = &config.AppState{Name: n}
		}
		out = append(out, s)
	}
	return out, nil
}

func capturePs(client *dc.Client, service string) (string, error) {
	cmd := exec.Command("docker", "compose", "-f", client.RootFile, "-p", client.Project, "ps", service)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// redirectBack honors htmx's HX-Request header: return 204+HX-Redirect for
// htmx callers, 303 for plain-HTML form posts.
func redirectBack(w http.ResponseWriter, r *http.Request, to string) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", to)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}
