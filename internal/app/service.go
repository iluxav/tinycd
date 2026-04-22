package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/iluxav/tinycd/internal/sh"
)

const serviceName = "tcd-ui.service"

func newServiceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Manage the tcd-ui systemd --user service",
	}
	cmd.AddCommand(newServiceInstallCmd(), newServiceUninstallCmd(), newServiceStatusCmd())
	return cmd
}

func newServiceInstallCmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install and enable the tcd-ui systemd --user service",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS != "linux" {
				return fmt.Errorf("service install is linux-only (systemd). on macOS, run `tcd ui` directly.")
			}
			if err := sh.Look("systemctl"); err != nil {
				return fmt.Errorf("systemd not available: %w", err)
			}
			bin, err := os.Executable()
			if err != nil {
				return err
			}
			bin, _ = filepath.Abs(bin)

			unitDir, err := userUnitDir()
			if err != nil {
				return err
			}
			if err := os.MkdirAll(unitDir, 0o755); err != nil {
				return err
			}
			unitPath := filepath.Join(unitDir, serviceName)
			unit := fmt.Sprintf(`[Unit]
Description=tcd web UI
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s ui --addr %s
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
`, bin, addr)
			if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
				return err
			}
			fmt.Printf("wrote %s\n", unitPath)

			if err := sh.Run(sh.Opts{}, "systemctl", "--user", "daemon-reload"); err != nil {
				return err
			}
			if err := sh.Run(sh.Opts{}, "systemctl", "--user", "enable", "--now", serviceName); err != nil {
				return err
			}
			fmt.Printf("✓ tcd-ui enabled and started\n")
			fmt.Printf("  open http://%s in your browser\n", addr)
			fmt.Printf("  status: systemctl --user status %s\n", serviceName)
			fmt.Printf("  logs:   journalctl --user -u %s -f\n", serviceName)

			// Remind about `loginctl enable-linger` so service survives logout.
			if out, err := exec.Command("loginctl", "show-user", os.Getenv("USER"), "-p", "Linger").Output(); err == nil {
				if string(out) != "Linger=yes\n" {
					fmt.Printf("\nnote: to keep tcd-ui running after you log out, run:\n")
					fmt.Printf("      sudo loginctl enable-linger $USER\n")
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:7070", "bind address for the UI")
	return cmd
}

func newServiceUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Disable and remove the tcd-ui systemd --user service",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS != "linux" {
				return fmt.Errorf("service uninstall is linux-only")
			}
			_ = sh.Run(sh.Opts{Quiet: true}, "systemctl", "--user", "disable", "--now", serviceName)
			unitDir, err := userUnitDir()
			if err != nil {
				return err
			}
			path := filepath.Join(unitDir, serviceName)
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
			_ = sh.Run(sh.Opts{Quiet: true}, "systemctl", "--user", "daemon-reload")
			fmt.Printf("✓ %s removed\n", serviceName)
			return nil
		},
	}
}

func newServiceStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show tcd-ui service status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return sh.Run(sh.Opts{}, "systemctl", "--user", "status", serviceName)
		},
	}
}

// userUnitDir returns the path to the user's systemd unit dir, typically
// ~/.config/systemd/user.
func userUnitDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user"), nil
}
