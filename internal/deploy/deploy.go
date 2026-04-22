package deploy

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/iluxav/tinycd/internal/compose"
	"github.com/iluxav/tinycd/internal/config"
	"github.com/iluxav/tinycd/internal/dc"
	gitpkg "github.com/iluxav/tinycd/internal/git"
)

// Options controls a deploy. Zero values fall back to defaults where sensible.
type Options struct {
	Repo    string
	Name    string
	Ref     string
	Port    int
	Scale   int
	Service string
	EnvFile string
	Aliases []string
}

// Deploy clones (or updates) the repo, writes the per-app override, adds an
// include entry, runs docker compose up, and writes state.json. It is the
// single implementation called by both the CLI and the UI.
func Deploy(cfg *config.Config, opts Options) (*config.AppState, error) {
	if opts.Repo == "" {
		return nil, fmt.Errorf("repo is required")
	}
	if opts.Port == 0 {
		opts.Port = 3000
	}
	if opts.Scale == 0 {
		opts.Scale = 1
	}

	repoURL, derivedName, err := compose.NormalizeRepoURL(opts.Repo)
	if err != nil {
		return nil, err
	}
	appName := opts.Name
	if appName == "" {
		appName = derivedName
	}

	appDir := cfg.AppDir(appName)
	repoDir := filepath.Join(appDir, "repo")

	if err := gitpkg.CloneOrPull(repoURL, repoDir, opts.Ref, cfg.SSHKeyPath); err != nil {
		return nil, err
	}
	commit, err := gitpkg.HeadCommit(repoDir)
	if err != nil {
		return nil, err
	}

	envTarget := filepath.Join(appDir, ".env")
	if opts.EnvFile != "" {
		if err := copyFile(opts.EnvFile, envTarget); err != nil {
			return nil, fmt.Errorf("copy env-file: %w", err)
		}
	}
	hasEnv := fileExists(envTarget)

	composePath, found := compose.DetectComposeFile(repoDir)
	if !found {
		generated := filepath.Join(repoDir, "compose.generated.yml")
		if err := generateFallbackCompose(generated, appName, opts.Port); err != nil {
			return nil, err
		}
		composePath = generated
	}
	parsed, err := compose.ParseComposeFile(composePath)
	if err != nil {
		return nil, err
	}
	primary, err := compose.ResolvePrimaryService(parsed, opts.Service)
	if err != nil {
		return nil, err
	}

	overridePath := filepath.Join(appDir, "override.yml")
	envRel := ""
	if hasEnv {
		envRel = ".env"
	}
	certResolver := ""
	if cfg.ACMEEmail != "" {
		certResolver = "le"
	}
	effectiveAliases := append([]string(nil), AutoAliases(appName, cfg.PublicDomains)...)
	effectiveAliases = append(effectiveAliases, opts.Aliases...)

	if err := compose.RenderOverride(compose.OverrideInput{
		AppName:      appName,
		PrimarySvc:   primary,
		Domain:       cfg.Domain,
		Port:         opts.Port,
		EnvFilePath:  envRel,
		NetworkName:  "tcd-proxy",
		CertResolver: certResolver,
		Aliases:      effectiveAliases,
	}, overridePath); err != nil {
		return nil, err
	}

	relCompose, err := filepath.Rel(cfg.StateDir, composePath)
	if err != nil {
		return nil, err
	}
	relOverride, err := filepath.Rel(cfg.StateDir, overridePath)
	if err != nil {
		return nil, err
	}
	if err := compose.AddInclude(cfg.RootComposeFile(), appName, []string{relCompose, relOverride}); err != nil {
		return nil, err
	}

	if err := dc.EnsureNetwork("tcd-proxy"); err != nil {
		return nil, fmt.Errorf("ensure tcd-proxy network: %w", err)
	}
	client := &dc.Client{
		RootFile: cfg.RootComposeFile(),
		Project:  "tcd",
		Env:      []string{"ACME_EMAIL=" + cfg.ACMEEmail},
	}
	scales := map[string]int{}
	if opts.Scale > 1 {
		scales[primary] = opts.Scale
	}
	if err := client.Up(scales); err != nil {
		return nil, err
	}

	state := &config.AppState{
		Name:         appName,
		Repo:         opts.Repo,
		RepoURL:      repoURL,
		Ref:          opts.Ref,
		Commit:       commit,
		Service:      primary,
		Port:         opts.Port,
		Scale:        opts.Scale,
		URL:          fmt.Sprintf("http://%s.%s", appName, cfg.Domain),
		Aliases:      effectiveAliases,
		EnvFile:      envTarget,
		ComposeFile:  composePath,
		OverrideFile: overridePath,
	}
	if cfg.ACMEEmail != "" {
		state.URL = fmt.Sprintf("https://%s.%s", appName, cfg.Domain)
	}
	if err := config.SaveState(appDir, state); err != nil {
		return nil, err
	}
	return state, nil
}

// AutoAliases derives <app>.<publicDomain> for each configured public domain.
func AutoAliases(appName string, publicDomains []string) []string {
	out := make([]string, 0, len(publicDomains))
	for _, pd := range publicDomains {
		if pd == "" {
			continue
		}
		out = append(out, appName+"."+pd)
	}
	return out
}

// Remove tears down an app: drops the root include entry, runs compose up to
// reconcile, and optionally purges its state directory.
func Remove(cfg *config.Config, name string, purge bool) error {
	if err := compose.RemoveInclude(cfg.RootComposeFile(), name); err != nil {
		return err
	}
	client := &dc.Client{
		RootFile: cfg.RootComposeFile(),
		Project:  "tcd",
		Env:      []string{"ACME_EMAIL=" + cfg.ACMEEmail},
	}
	// Best-effort: compose up to tear down removed services.
	_ = client.Up(nil)
	if purge {
		if err := os.RemoveAll(cfg.AppDir(name)); err != nil {
			return fmt.Errorf("purge %s: %w", cfg.AppDir(name), err)
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func generateFallbackCompose(path, appName string, port int) error {
	content := fmt.Sprintf(`# managed by tcd — generated because no compose file was found in the repo
services:
  %s:
    build: .
    expose:
      - "%d"
    environment:
      PORT: "%d"
`, appName, port, port)
	return os.WriteFile(path, []byte(content), 0o644)
}
