package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/iluxav/tinycd/internal/compose"
	"github.com/iluxav/tinycd/internal/config"
	"github.com/iluxav/tinycd/internal/dc"
	"github.com/iluxav/tinycd/internal/sh"
)

func newInitCmd() *cobra.Command {
	var domain, email string
	var stateDir string
	var publicDomains []string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize tcd on this host",
		RunE: func(cmd *cobra.Command, args []string) error {
			if domain == "" {
				return fmt.Errorf("--domain is required")
			}
			if err := dc.Verify(); err != nil {
				return err
			}
			cfgDir, err := config.DefaultConfigDir()
			if err != nil {
				return err
			}
			if err := os.MkdirAll(cfgDir, 0o755); err != nil {
				return err
			}

			if stateDir == "" {
				stateDir = config.DefaultStateDir()
			}
			appsDir := filepath.Join(stateDir, "apps")
			if err := os.MkdirAll(appsDir, 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", appsDir, err)
			}

			if err := dc.EnsureNetwork("tcd-proxy"); err != nil {
				return fmt.Errorf("create tcd-proxy network: %w", err)
			}

			// SSH key
			keyPath := filepath.Join(cfgDir, "id_ed25519")
			pubPath := keyPath + ".pub"
			if _, err := os.Stat(keyPath); os.IsNotExist(err) {
				if err := sh.Look("ssh-keygen"); err != nil {
					return err
				}
				if err := sh.Run(sh.Opts{}, "ssh-keygen", "-t", "ed25519", "-N", "", "-f", keyPath, "-C", "tcd@"+domain); err != nil {
					return err
				}
			}

			// Root compose + traefik dir.
			traefikDir := filepath.Join(stateDir, "traefik")
			if err := os.MkdirAll(traefikDir, 0o755); err != nil {
				return err
			}
			if email != "" {
				acme := filepath.Join(traefikDir, "acme.json")
				if _, err := os.Stat(acme); os.IsNotExist(err) {
					if err := os.WriteFile(acme, []byte("{}"), 0o600); err != nil {
						return err
					}
				}
			}
			rootCompose := filepath.Join(stateDir, "compose.yml")
			if _, err := os.Stat(rootCompose); os.IsNotExist(err) {
				data, err := compose.RenderRootCompose(compose.RootComposeInput{ACMEEmail: email})
				if err != nil {
					return err
				}
				if err := os.WriteFile(rootCompose, data, 0o644); err != nil {
					return err
				}
			}

			cfg := &config.Config{
				Domain:        domain,
				PublicDomains: dedupe(publicDomains),
				ACMEEmail:     email,
				AppsDir:       appsDir,
				StateDir:      stateDir,
				SSHKeyPath:    keyPath,
			}
			if err := config.Save(cfg); err != nil {
				return err
			}

			suggestEtunl(cfg)

			pub, err := os.ReadFile(pubPath)
			if err != nil {
				return fmt.Errorf("read pubkey: %w", err)
			}
			fmt.Println("tcd initialized.")
			fmt.Printf("  domain:    %s\n", domain)
			if len(cfg.PublicDomains) > 0 {
				fmt.Printf("  public:    %v\n", cfg.PublicDomains)
			}
			fmt.Printf("  state:     %s\n", stateDir)
			fmt.Printf("  ssh key:   %s\n", keyPath)
			fmt.Println()
			fmt.Println("Add this deploy key to GitHub (Settings → Deploy keys, one per repo):")
			fmt.Println()
			fmt.Print(string(pub))
			fmt.Println()
			fmt.Println("Optional: install the web UI as a service — `tcd service install` (linux/systemd).")
			return nil
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "base domain for apps (app.<domain>)")
	cmd.Flags().StringArrayVar(&publicDomains, "public-domain", nil, "extra public base domain — every deploy auto-aliases <app>.<public-domain> (repeatable)")
	cmd.Flags().StringVar(&email, "acme-email", "", "email for Let's Encrypt (enables TLS)")
	cmd.Flags().StringVar(&stateDir, "state-dir", "", "override state dir (default: /var/lib/tcd or ~/.local/share/tcd)")
	return cmd
}

func dedupe(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// suggestEtunl prints a hint if an etunl client config is present but its
// server isn't in the public_domains list. No side effects, no prompts.
func suggestEtunl(cfg *config.Config) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	data, err := os.ReadFile(filepath.Join(home, ".etunl", "config.yaml"))
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		s := strings.TrimSpace(line)
		if !strings.HasPrefix(s, "server:") {
			continue
		}
		server := strings.Trim(strings.TrimSpace(strings.TrimPrefix(s, "server:")), `"'`)
		if server == "" {
			return
		}
		for _, pd := range cfg.PublicDomains {
			if pd == server {
				return
			}
		}
		fmt.Printf("\nhint: detected etunl config with server=%s\n", server)
		fmt.Printf("      re-run with --public-domain %s to auto-route deploys through it\n", server)
		return
	}
}
