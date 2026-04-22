package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Domain        string   `yaml:"domain"`
	PublicDomains []string `yaml:"public_domains,omitempty"`
	ACMEEmail     string   `yaml:"acme_email,omitempty"`
	AppsDir       string   `yaml:"apps_dir"`
	StateDir      string   `yaml:"state_dir"`
	SSHKeyPath    string   `yaml:"ssh_key_path"`
}

// DefaultConfigDir returns ~/.config/tcd
func DefaultConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "tcd"), nil
}

// DefaultStateDir returns /var/lib/tcd, or ~/.local/share/tcd if the user can't write to /var/lib.
func DefaultStateDir() string {
	const sys = "/var/lib/tcd"
	// Probe write access to parent.
	if err := checkWritable("/var/lib"); err == nil {
		return sys
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return sys
	}
	return filepath.Join(home, ".local", "share", "tcd")
}

func checkWritable(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("not a directory")
	}
	// Try creating a tempfile.
	f, err := os.CreateTemp(dir, ".tcd-probe-")
	if err != nil {
		return err
	}
	f.Close()
	os.Remove(f.Name())
	return nil
}

func ConfigPath() (string, error) {
	dir, err := DefaultConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yml"), nil
}

func Load() (*Config, error) {
	p, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("tcd not initialized — run `tcd init --domain <domain>`")
		}
		return nil, err
	}
	c := &Config{}
	if err := yaml.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	return c, nil
}

func Save(c *Config) error {
	p, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// AppDir returns the directory for a given app inside state dir.
func (c *Config) AppDir(name string) string {
	return filepath.Join(c.AppsDir, name)
}

func (c *Config) RootComposeFile() string {
	return filepath.Join(c.StateDir, "compose.yml")
}

func (c *Config) TraefikDir() string {
	return filepath.Join(c.StateDir, "traefik")
}
