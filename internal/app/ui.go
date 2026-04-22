package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/iluxav/tinycd/internal/auth"
	"github.com/iluxav/tinycd/internal/sh"
	"github.com/iluxav/tinycd/internal/ui"
)

func newUICmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Run the web UI (localhost-only by default)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := auth.Load(); err != nil {
				if errors.Is(err, auth.ErrNotConfigured) {
					return fmt.Errorf("auth not configured — run `tcd admin set-password admin` first")
				}
				return err
			}
			// Startup probe: if this process can't reach docker, fail loudly now
			// with actionable guidance instead of breaking on the first user action.
			if err := sh.Run(sh.Opts{Quiet: true}, "docker", "info"); err != nil {
				return fmt.Errorf("cannot reach docker daemon: %w\n\nhint: this process needs access to the docker socket. If you recently ran `usermod -aG docker`, systemd --user needs a fresh session: `sudo systemctl restart user@$(id -u).service` (or log out fully and back in)", err)
			}
			srv, err := ui.New()
			if err != nil {
				return err
			}
			httpSrv := &http.Server{
				Addr:              addr,
				Handler:           srv.Handler(),
				ReadHeaderTimeout: 10 * time.Second,
			}
			fmt.Printf("tcd ui listening on http://%s\n", addr)

			// Graceful shutdown on SIGINT/SIGTERM.
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			errCh := make(chan error, 1)
			go func() { errCh <- httpSrv.ListenAndServe() }()

			select {
			case err := <-errCh:
				if err != nil && !errors.Is(err, http.ErrServerClosed) {
					return err
				}
				return nil
			case <-sigCh:
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				return httpSrv.Shutdown(ctx)
			}
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:7070", "bind address (use 0.0.0.0:7070 at your own risk — no auth)")
	return cmd
}
