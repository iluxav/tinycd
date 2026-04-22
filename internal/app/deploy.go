package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/iluxav/tinycd/internal/compose"
	"github.com/iluxav/tinycd/internal/config"
	"github.com/iluxav/tinycd/internal/dc"
	gitpkg "github.com/iluxav/tinycd/internal/git"
)

func newDeployCmd() *cobra.Command {
	var (
		name    string
		ref     string
		port    int
		scale   int
		service string
		envFile string
		aliases []string
	)
	cmd := &cobra.Command{
		Use:   "deploy <repo>",
		Short: "Deploy or update an app from a GitHub repo",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			repoInput := args[0]
			repoURL, derivedName, err := compose.NormalizeRepoURL(repoInput)
			if err != nil {
				return err
			}
			appName := name
			if appName == "" {
				appName = derivedName
			}

			appDir := cfg.AppDir(appName)
			repoDir := filepath.Join(appDir, "repo")

			fmt.Printf("→ deploying %s as %s\n", repoURL, appName)

			if err := gitpkg.CloneOrPull(repoURL, repoDir, ref, cfg.SSHKeyPath); err != nil {
				return err
			}
			commit, err := gitpkg.HeadCommit(repoDir)
			if err != nil {
				return err
			}

			// Env file
			envTarget := filepath.Join(appDir, ".env")
			if envFile != "" {
				if err := copyFile(envFile, envTarget); err != nil {
					return fmt.Errorf("copy env-file: %w", err)
				}
			}
			hasEnv := fileExists(envTarget)

			// Compose detection / generation
			composePath, found := compose.DetectComposeFile(repoDir)
			if !found {
				generated := filepath.Join(repoDir, "compose.generated.yml")
				if err := generateFallbackCompose(generated, appName, port); err != nil {
					return err
				}
				composePath = generated
			}
			parsed, err := compose.ParseComposeFile(composePath)
			if err != nil {
				return err
			}
			primary, err := compose.ResolvePrimaryService(parsed, service)
			if err != nil {
				return err
			}

			// Override
			overridePath := filepath.Join(appDir, "override.yml")
			// env_file path is relative to override.yml's dir (the app dir), so just ".env".
			envRel := ""
			if hasEnv {
				envRel = ".env"
			}
			certResolver := ""
			if cfg.ACMEEmail != "" {
				certResolver = "le"
			}
			if err := compose.RenderOverride(compose.OverrideInput{
				AppName:      appName,
				PrimarySvc:   primary,
				Domain:       cfg.Domain,
				Port:         port,
				EnvFilePath:  envRel,
				NetworkName:  "tcd-proxy",
				CertResolver: certResolver,
				Aliases:      aliases,
			}, overridePath); err != nil {
				return err
			}

			// Root include
			relCompose, err := filepath.Rel(cfg.StateDir, composePath)
			if err != nil {
				return err
			}
			relOverride, err := filepath.Rel(cfg.StateDir, overridePath)
			if err != nil {
				return err
			}
			if err := compose.AddInclude(cfg.RootComposeFile(), appName, []string{relCompose, relOverride}); err != nil {
				return err
			}

			// docker compose up
			if err := dc.EnsureNetwork("tcd-proxy"); err != nil {
				return fmt.Errorf("ensure tcd-proxy network: %w", err)
			}
			client := &dc.Client{
				RootFile: cfg.RootComposeFile(),
				Project:  "tcd",
				Env:      []string{"ACME_EMAIL=" + cfg.ACMEEmail},
			}
			scales := map[string]int{}
			if scale > 1 {
				scales[primary] = scale
			}
			if err := client.Up(scales); err != nil {
				return err
			}

			// State
			state := &config.AppState{
				Name:         appName,
				Repo:         repoInput,
				RepoURL:      repoURL,
				Ref:          ref,
				Commit:       commit,
				Service:      primary,
				Port:         port,
				Scale:        scale,
				URL:          fmt.Sprintf("http://%s.%s", appName, cfg.Domain),
				Aliases:      aliases,
				EnvFile:      envTarget,
				ComposeFile:  composePath,
				OverrideFile: overridePath,
			}
			if cfg.ACMEEmail != "" {
				state.URL = fmt.Sprintf("https://%s.%s", appName, cfg.Domain)
			}
			if err := config.SaveState(appDir, state); err != nil {
				return err
			}

			fmt.Printf("✓ %s deployed at %s (commit %s)\n", appName, state.URL, shortCommit(commit))
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "override app name (default: repo basename)")
	cmd.Flags().StringVar(&ref, "ref", "", "branch, tag, or commit to check out (default: remote HEAD)")
	cmd.Flags().IntVar(&port, "port", 3000, "internal app port exposed to Traefik")
	cmd.Flags().IntVar(&scale, "scale", 1, "number of replicas")
	cmd.Flags().StringVar(&service, "service", "", "primary service name (overrides auto-detection)")
	cmd.Flags().StringVar(&envFile, "env-file", "", "path to .env file to install for this app")
	cmd.Flags().StringArrayVar(&aliases, "alias", nil, "additional hostname to route to this app (repeatable, e.g. --alias hd.etunl.com)")
	return cmd
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

func shortCommit(c string) string {
	if len(c) >= 7 {
		return c[:7]
	}
	return c
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
