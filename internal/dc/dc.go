package dc

import (
	"fmt"

	"github.com/iluxav/tinycd/internal/sh"
)

// Client wraps `docker compose -f <root> -p <project>`.
type Client struct {
	RootFile string
	Project  string
	Env      []string
}

func (c *Client) base() []string {
	return []string{"compose", "-f", c.RootFile, "-p", c.Project}
}

func (c *Client) run(args ...string) error {
	return sh.Run(sh.Opts{Env: c.Env}, "docker", append(c.base(), args...)...)
}

func (c *Client) capture(args ...string) (string, error) {
	return sh.Capture(sh.Opts{Env: c.Env}, "docker", append(c.base(), args...)...)
}

// Up runs `up -d --build` with optional per-service --scale values.
func (c *Client) Up(scales map[string]int) error {
	args := []string{"up", "-d", "--build", "--remove-orphans"}
	for svc, n := range scales {
		args = append(args, "--scale", fmt.Sprintf("%s=%d", svc, n))
	}
	return c.run(args...)
}

func (c *Client) Restart(services ...string) error {
	args := append([]string{"restart"}, services...)
	return c.run(args...)
}

func (c *Client) Stop(services ...string) error {
	args := append([]string{"stop"}, services...)
	return c.run(args...)
}

// Down stops and removes containers for the project.
func (c *Client) Down() error {
	return c.run("down")
}

func (c *Client) Logs(follow bool, tail string, services ...string) error {
	args := []string{"logs", "--tail", tail}
	if follow {
		args = append(args, "-f")
	}
	args = append(args, services...)
	return c.run(args...)
}

func (c *Client) PsJSON() (string, error) {
	return c.capture("ps", "--format", "json")
}

func (c *Client) PsTable(services ...string) error {
	args := append([]string{"ps"}, services...)
	return c.run(args...)
}

// Verify checks that docker and `docker compose` are usable.
func Verify() error {
	if err := sh.Look("docker"); err != nil {
		return err
	}
	if err := sh.Run(sh.Opts{Quiet: true}, "docker", "compose", "version"); err != nil {
		return fmt.Errorf("`docker compose` not available: %w", err)
	}
	return nil
}

// EnsureNetwork creates a docker network if it doesn't already exist.
func EnsureNetwork(name string) error {
	// Fast path: inspect succeeds → already exists and we have docker access.
	if err := sh.Run(sh.Opts{Quiet: true}, "docker", "network", "inspect", name); err == nil {
		return nil
	}
	// Try to create. On failure, check whether docker itself is reachable; if
	// not, the most common cause is group membership not applied to this
	// process (systemd --user started before `usermod -aG docker`).
	if err := sh.Run(sh.Opts{Quiet: true}, "docker", "network", "create", name); err != nil {
		if probeErr := sh.Run(sh.Opts{Quiet: true}, "docker", "info"); probeErr != nil {
			return fmt.Errorf("%w\n\nhint: this process can't talk to the docker daemon. If you recently added your user to the `docker` group, systemd --user needs a fresh session: run `sudo systemctl restart user@$(id -u).service` or log out fully and back in", probeErr)
		}
		return err
	}
	return nil
}
