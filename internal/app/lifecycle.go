package app

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/iluxav/tinycd/internal/compose"
	"github.com/iluxav/tinycd/internal/config"
	"github.com/iluxav/tinycd/internal/dc"
)

func newClient(cfg *config.Config) *dc.Client {
	return &dc.Client{
		RootFile: cfg.RootComposeFile(),
		Project:  "tcd",
		Env:      []string{"ACME_EMAIL=" + cfg.ACMEEmail},
	}
}

func loadApp(cfg *config.Config, name string) (*config.AppState, error) {
	s, err := config.LoadState(cfg.AppDir(name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("app %q not found", name)
		}
		return nil, err
	}
	return s, nil
}

func newRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart <app>",
		Short: "Restart an app",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			state, err := loadApp(cfg, args[0])
			if err != nil {
				return err
			}
			return newClient(cfg).Restart(state.Service)
		},
	}
}

func newStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop <app>",
		Short: "Stop an app",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			state, err := loadApp(cfg, args[0])
			if err != nil {
				return err
			}
			return newClient(cfg).Stop(state.Service)
		},
	}
}

func newRmCmd() *cobra.Command {
	var purge bool
	cmd := &cobra.Command{
		Use:   "rm <app>",
		Short: "Remove an app",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			name := args[0]
			// Even if state is missing, attempt to remove include + purge dir.
			if err := compose.RemoveInclude(cfg.RootComposeFile(), name); err != nil {
				return err
			}
			// Bring the project up without the removed app so its containers get torn down.
			client := newClient(cfg)
			if err := client.Up(nil); err != nil {
				// Best-effort: still try to purge.
				fmt.Fprintf(os.Stderr, "warning: docker compose up after rm failed: %v\n", err)
			}
			if purge {
				appDir := cfg.AppDir(name)
				if err := os.RemoveAll(appDir); err != nil {
					return fmt.Errorf("purge %s: %w", appDir, err)
				}
				fmt.Printf("✓ %s removed and purged\n", name)
			} else {
				fmt.Printf("✓ %s removed (use --purge to also delete %s)\n", name, cfg.AppDir(name))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&purge, "purge", false, "also delete repo + state directory")
	return cmd
}

func newLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List deployed apps",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			names, err := compose.ListIncludes(cfg.RootComposeFile())
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tREPO\tREF\tURL\tSCALE\tSTATUS")
			for _, n := range names {
				s, err := config.LoadState(cfg.AppDir(n))
				if err != nil {
					fmt.Fprintf(w, "%s\t?\t?\t?\t?\t<no state>\n", n)
					continue
				}
				status := "up"
				ref := s.Ref
				if ref == "" {
					ref = "HEAD"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\n", s.Name, s.Repo, ref, s.URL, s.Scale, status)
			}
			return w.Flush()
		},
	}
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <app>",
		Short: "Show app details and container status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			s, err := loadApp(cfg, args[0])
			if err != nil {
				return err
			}
			fmt.Printf("name:         %s\n", s.Name)
			fmt.Printf("repo:         %s\n", s.Repo)
			fmt.Printf("repo URL:     %s\n", s.RepoURL)
			fmt.Printf("ref:          %s\n", s.Ref)
			fmt.Printf("commit:       %s\n", s.Commit)
			fmt.Printf("url:          %s\n", s.URL)
			for _, a := range s.Aliases {
				fmt.Printf("alias:        %s\n", a)
			}
			fmt.Printf("port:         %d\n", s.Port)
			fmt.Printf("scale:        %d\n", s.Scale)
			fmt.Printf("service:      %s\n", s.Service)
			fmt.Printf("compose:      %s\n", s.ComposeFile)
			fmt.Printf("override:     %s\n", s.OverrideFile)
			fmt.Printf("repo path:    %s\n", filepath.Join(cfg.AppDir(s.Name), "repo"))
			fmt.Println()
			return newClient(cfg).PsTable(s.Service)
		},
	}
}

func newLogsCmd() *cobra.Command {
	var follow bool
	var tail string
	cmd := &cobra.Command{
		Use:   "logs <app>",
		Short: "Show container logs for an app",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			s, err := loadApp(cfg, args[0])
			if err != nil {
				return err
			}
			return newClient(cfg).Logs(follow, tail, s.Service)
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow log output")
	cmd.Flags().StringVar(&tail, "tail", "200", "number of lines to show from end of logs")
	return cmd
}
