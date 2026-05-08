package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// AppState tracks everything tcd knows about a deployed app.
type AppState struct {
	Name         string        `json:"name"`
	Repo         string        `json:"repo"`
	RepoURL      string        `json:"repo_url"`
	Ref          string        `json:"ref"`
	Commit       string        `json:"commit"`
	Service      string        `json:"service"`
	Port         int           `json:"port"`
	Scale        int           `json:"scale"`
	URL          string        `json:"url"`
	Aliases      []string      `json:"aliases,omitempty"`
	EnvFile      string        `json:"env_file,omitempty"`
	ComposeFile  string        `json:"compose_file"`
	OverrideFile string        `json:"override_file"`
	VolumeMounts []VolumeMount `json:"volume_mounts,omitempty"`
}

// VolumeMount records a host-backed bind mount that tcd auto-attached to a
// service because the service's image declared VOLUME for that path.
type VolumeMount struct {
	Service   string `json:"service"`
	HostPath  string `json:"host_path"`
	MountPath string `json:"mount_path"`
}

func StatePath(appDir string) string {
	return filepath.Join(appDir, "state.json")
}

func LoadState(appDir string) (*AppState, error) {
	p := StatePath(appDir)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	s := &AppState{}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	return s, nil
}

func SaveState(appDir string, s *AppState) error {
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(StatePath(appDir), data, 0o644)
}
