package dc

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/iluxav/tinycd/internal/sh"
)

// Client wraps `docker compose -f <root> -p <project>`.
type Client struct {
	RootFile string
	Project  string
	Env      []string

	// userCache memoizes ResolveUser results per (image, user) so we don't
	// pay the docker-run cost twice in one deploy.
	userCache map[string]userIDs
}

type userIDs struct {
	UID, GID int
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

// Up runs `up -d --remove-orphans` with optional per-service --scale values.
// Build() and Pull() should be called first; Up no longer rebuilds.
func (c *Client) Up(scales map[string]int) error {
	args := []string{"up", "-d", "--remove-orphans"}
	for svc, n := range scales {
		args = append(args, "--scale", fmt.Sprintf("%s=%d", svc, n))
	}
	return c.run(args...)
}

// Build runs `docker compose build` for all services that have a build: stanza.
func (c *Client) Build() error {
	return c.run("build")
}

// Pull runs `docker compose pull` to fetch all image: refs. --ignore-pull-failures
// keeps the call from erroring when a service has only build: (compose tries
// to pull anyway and treats the failure as fatal otherwise).
func (c *Client) Pull() error {
	return c.run("pull", "--ignore-pull-failures")
}

// ServiceImages returns the resolved image ref per service from `docker compose
// config --format json`. Works for both image:-only services and build:
// services (compose synthesizes a name like <project>-<service>).
func (c *Client) ServiceImages() (map[string]string, error) {
	out, err := c.capture("config", "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("docker compose config: %w", err)
	}
	var doc struct {
		Services map[string]struct {
			Image string `json:"image"`
		} `json:"services"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		return nil, fmt.Errorf("parse compose config: %w", err)
	}
	images := make(map[string]string, len(doc.Services))
	for name, svc := range doc.Services {
		if svc.Image != "" {
			images[name] = svc.Image
		}
	}
	return images, nil
}

// InspectImage returns the in-container paths declared by VOLUME directives
// and the image's runtime User string (may be empty, numeric, or named).
func (c *Client) InspectImage(image string) (volumes []string, user string, err error) {
	out, err := sh.Capture(sh.Opts{Env: c.Env}, "docker", "image", "inspect", image, "--format", "{{json .Config}}")
	if err != nil {
		return nil, "", fmt.Errorf("docker image inspect %s: %w", image, err)
	}
	var cfg struct {
		Volumes map[string]struct{} `json:"Volumes"`
		User    string              `json:"User"`
	}
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		return nil, "", fmt.Errorf("parse image config %s: %w", image, err)
	}
	for v := range cfg.Volumes {
		volumes = append(volumes, v)
	}
	return volumes, cfg.User, nil
}

// ResolveUser maps an image's User string to numeric UID:GID. Empty → 0:0.
// Numeric strings ("1001" or "1001:1001") parse directly. Named users
// ("nextjs:nodejs") require running the image to query /etc/passwd via id(1).
// Results are cached for the lifetime of the Client.
func (c *Client) ResolveUser(image, user string) (uid, gid int, err error) {
	if user == "" {
		return 0, 0, nil
	}
	if c.userCache == nil {
		c.userCache = map[string]userIDs{}
	}
	key := image + "\x00" + user
	if cached, ok := c.userCache[key]; ok {
		return cached.UID, cached.GID, nil
	}

	if u, g, ok := parseNumericUser(user); ok {
		c.userCache[key] = userIDs{u, g}
		return u, g, nil
	}

	// Named user: run a throwaway container to look up the numeric IDs.
	out, err := sh.Capture(sh.Opts{Env: c.Env}, "docker", "run", "--rm", "--entrypoint", "sh", image, "-c", "id -u "+shellEscape(strings.SplitN(user, ":", 2)[0])+"; id -g "+shellEscape(strings.SplitN(user, ":", 2)[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("resolve user %q in image %s: %w", user, image, err)
	}
	lines := strings.Fields(out)
	if len(lines) < 2 {
		return 0, 0, fmt.Errorf("unexpected id output for user %q in image %s: %q", user, image, out)
	}
	uid, err = strconv.Atoi(lines[0])
	if err != nil {
		return 0, 0, fmt.Errorf("parse uid %q: %w", lines[0], err)
	}
	gid, err = strconv.Atoi(lines[1])
	if err != nil {
		return 0, 0, fmt.Errorf("parse gid %q: %w", lines[1], err)
	}
	c.userCache[key] = userIDs{uid, gid}
	return uid, gid, nil
}

func parseNumericUser(s string) (uid, gid int, ok bool) {
	parts := strings.SplitN(s, ":", 2)
	u, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	g := u
	if len(parts) == 2 {
		g, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, false
		}
	}
	return u, g, true
}

// shellEscape produces a single-quoted string safe to embed in `sh -c`.
func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
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
