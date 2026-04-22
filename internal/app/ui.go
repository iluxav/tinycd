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
