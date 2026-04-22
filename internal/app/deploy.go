package app

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/iluxav/tinycd/internal/config"
	"github.com/iluxav/tinycd/internal/deploy"
)

func newDeployCmd() *cobra.Command {
	var opts deploy.Options
	cmd := &cobra.Command{
		Use:   "deploy <repo>",
		Short: "Deploy or update an app from a GitHub repo",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			opts.Repo = args[0]
			fmt.Printf("→ deploying %s\n", opts.Repo)
			state, err := deploy.Deploy(cfg, opts)
			if err != nil {
				return err
			}
			short := state.Commit
			if len(short) > 7 {
				short = short[:7]
			}
			fmt.Printf("✓ %s deployed at %s (commit %s)\n", state.Name, state.URL, short)
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.Name, "name", "", "override app name (default: repo basename)")
	cmd.Flags().StringVar(&opts.Ref, "ref", "", "branch, tag, or commit to check out (default: remote HEAD)")
	cmd.Flags().IntVar(&opts.Port, "port", 3000, "internal app port exposed to Traefik")
	cmd.Flags().IntVar(&opts.Scale, "scale", 1, "number of replicas")
	cmd.Flags().StringVar(&opts.Service, "service", "", "primary service name (overrides auto-detection)")
	cmd.Flags().StringVar(&opts.EnvFile, "env-file", "", "path to .env file to install for this app")
	cmd.Flags().StringArrayVar(&opts.Aliases, "alias", nil, "additional hostname to route to this app (repeatable, e.g. --alias hd.etunl.com)")
	return cmd
}
