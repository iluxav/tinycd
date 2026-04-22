package dc

import (
	"fmt"

	"github.com/iluxa/tinycd/internal/sh"
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
