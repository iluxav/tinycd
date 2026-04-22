package sh

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)


type Opts struct {
	Dir    string
	Env    []string
	Stdin  io.Reader
	Quiet  bool
}

func Run(opts Opts, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = opts.Dir
	if len(opts.Env) > 0 {
		cmd.Env = append(os.Environ(), opts.Env...)
	}
	cmd.Stdin = opts.Stdin

	// Always capture stderr so errors carry useful context (e.g. docker
	// permission denied) back up to callers. Also tee to os.Stderr so human
	// operators watching the process see the same stream in real time.
	var errBuf bytes.Buffer
	if opts.Quiet {
		cmd.Stdout = io.Discard
		cmd.Stderr = &errBuf
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = io.MultiWriter(os.Stderr, &errBuf)
	}
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errBuf.String())
		if msg != "" {
			return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, msg)
		}
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func Capture(opts Opts, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = opts.Dir
	if len(opts.Env) > 0 {
		cmd.Env = append(os.Environ(), opts.Env...)
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return strings.TrimRight(out.String(), "\n"), nil
}

func Look(name string) error {
	_, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("%s not found in PATH", name)
	}
	return nil
}
