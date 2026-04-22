package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/iluxav/tinycd/internal/auth"
)

func newAdminCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Manage admin credentials for the web UI",
	}
	cmd.AddCommand(newAdminSetPasswordCmd(), newAdminListUsersCmd())
	return cmd
}

func newAdminSetPasswordCmd() *cobra.Command {
	var fromStdin bool
	cmd := &cobra.Command{
		Use:   "set-password [username]",
		Short: "Set or change a user's password (default user: admin)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			user := "admin"
			if len(args) == 1 {
				user = args[0]
			}

			// Load or initialize.
			f, err := auth.Load()
			if err != nil && !errors.Is(err, auth.ErrNotConfigured) {
				return err
			}
			if f == nil {
				f = &auth.File{}
			}

			pw, err := readPassword(fromStdin)
			if err != nil {
				return err
			}
			if err := f.SetPassword(user, pw); err != nil {
				return err
			}
			if err := f.Save(); err != nil {
				return err
			}
			p, _ := auth.Path()
			fmt.Printf("✓ password set for user %q\n  stored at %s\n", user, p)
			return nil
		},
	}
	cmd.Flags().BoolVar(&fromStdin, "stdin", false, "read password from stdin (for scripting) instead of prompting")
	return cmd
}

func newAdminListUsersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list-users",
		Short: "List configured UI users",
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := auth.Load()
			if err != nil {
				return err
			}
			for _, u := range f.Users {
				fmt.Println(u.Name)
			}
			return nil
		},
	}
}

func readPassword(fromStdin bool) (string, error) {
	if fromStdin {
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		return strings.TrimRight(strings.TrimRight(string(raw), "\n"), "\r"), nil
	}
	// Interactive prompt — terminal required.
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New("no terminal attached; use --stdin to supply the password")
	}
	fmt.Print("new password: ")
	pw1, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", err
	}
	fmt.Print("confirm password: ")
	pw2, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", err
	}
	if string(pw1) != string(pw2) {
		return "", errors.New("passwords do not match")
	}
	return string(pw1), nil
}

