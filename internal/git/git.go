package git

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/iluxav/tinycd/internal/sh"
)

// CloneOrPull clones to dir if missing, otherwise fetches + resets to origin/<ref>.
// If ref is empty, uses the remote HEAD.
func CloneOrPull(url, dir, ref string, sshKey string) error {
	env := []string{}
	if sshKey != "" {
		env = append(env, "GIT_SSH_COMMAND=ssh -i "+sshKey+" -o StrictHostKeyChecking=accept-new")
	}

	if _, err := os.Stat(filepath.Join(dir, ".git")); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return err
		}
		if err := sh.Run(sh.Opts{Env: env}, "git", "clone", url, dir); err != nil {
			return err
		}
	} else {
		if err := sh.Run(sh.Opts{Dir: dir, Env: env}, "git", "fetch", "--all", "--prune"); err != nil {
			return err
		}
	}

	if ref == "" {
		var err error
		ref, err = DefaultBranch(dir, env)
		if err != nil {
			return err
		}
	}

	if err := sh.Run(sh.Opts{Dir: dir, Env: env}, "git", "checkout", ref); err != nil {
		return err
	}
	// Best-effort reset to origin/<ref> if it's a branch.
	_ = sh.Run(sh.Opts{Dir: dir, Env: env, Quiet: true}, "git", "reset", "--hard", "origin/"+ref)
	return nil
}

func DefaultBranch(dir string, env []string) (string, error) {
	out, err := sh.Capture(sh.Opts{Dir: dir, Env: env}, "git", "symbolic-ref", "refs/remotes/origin/HEAD")
	if err == nil {
		// refs/remotes/origin/main → main
		parts := strings.Split(strings.TrimSpace(out), "/")
		return parts[len(parts)-1], nil
	}
	// Fallback: main, then master.
	for _, b := range []string{"main", "master"} {
		if err := sh.Run(sh.Opts{Dir: dir, Env: env, Quiet: true}, "git", "rev-parse", "--verify", "origin/"+b); err == nil {
			return b, nil
		}
	}
	return "", err
}

func HeadCommit(dir string) (string, error) {
	return sh.Capture(sh.Opts{Dir: dir}, "git", "rev-parse", "HEAD")
}
