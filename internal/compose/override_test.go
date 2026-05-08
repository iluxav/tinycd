package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderOverride_WithEnvAndTLS(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "override.yml")
	err := RenderOverride(OverrideInput{
		AppName:      "app1",
		PrimarySvc:   "web",
		Domain:       "example.com",
		Port:         3000,
		EnvFilePath:  ".env",
		NetworkName:  "tcd-proxy",
		CertResolver: "le",
	}, out)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)

	must := []string{
		"web:",
		"tcd-proxy",
		"traefik.enable",
		"Host(`app1.example.com`)",
		"loadbalancer.server.port",
		"3000",
		"certresolver",
		"env_file",
	}
	for _, want := range must {
		if !strings.Contains(s, want) {
			t.Errorf("override output missing %q\n---\n%s", want, s)
		}
	}
}

func TestRenderOverride_WithAliases(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "override.yml")
	err := RenderOverride(OverrideInput{
		AppName:     "app1",
		PrimarySvc:  "web",
		Domain:      "example.com",
		Port:        3000,
		NetworkName: "tcd-proxy",
		Aliases:     []string{"hd.etunl.com", "foo.bar.test"},
	}, out)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	s := string(data)
	for _, want := range []string{
		"Host(`app1.example.com`)",
		"Host(`hd.etunl.com`)",
		"Host(`foo.bar.test`)",
		"||",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("override output missing %q\n---\n%s", want, s)
		}
	}
}

func TestBuildHostRule(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"a"}, "Host(`a`)"},
		{[]string{"a", "b"}, "Host(`a`) || Host(`b`)"},
		{[]string{"a", "", "b"}, "Host(`a`) || Host(`b`)"},
	}
	for _, c := range cases {
		if got := buildHostRule(c.in); got != c.want {
			t.Errorf("buildHostRule(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRenderOverride_AutoVolumes(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "override.yml")
	err := RenderOverride(OverrideInput{
		AppName:     "myapp",
		PrimarySvc:  "web",
		Domain:      "example.com",
		Port:        3000,
		NetworkName: "tcd-proxy",
		AutoVolumes: []AutoVolume{
			{Service: "web", HostPath: "/var/lib/tcd/myapp/volumes/web/data", MountPath: "/data"},
			{Service: "db", HostPath: "/var/lib/tcd/myapp/volumes/db/var/lib/postgresql/data", MountPath: "/var/lib/postgresql/data"},
		},
	}, out)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	s := string(data)
	for _, want := range []string{
		"/var/lib/tcd/myapp/volumes/web/data:/data",
		"/var/lib/tcd/myapp/volumes/db/var/lib/postgresql/data:/var/lib/postgresql/data",
		"db:", // non-primary service entry must appear
	} {
		if !strings.Contains(s, want) {
			t.Errorf("override output missing %q\n---\n%s", want, s)
		}
	}
}

func TestRenderOverride_NoTLS_NoEnv(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "override.yml")
	err := RenderOverride(OverrideInput{
		AppName:     "app1",
		PrimarySvc:  "web",
		Domain:      "example.com",
		Port:        8080,
		NetworkName: "tcd-proxy",
	}, out)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	s := string(data)
	if strings.Contains(s, "certresolver") {
		t.Errorf("expected no certresolver without CertResolver set:\n%s", s)
	}
	if strings.Contains(s, "env_file") {
		t.Errorf("expected no env_file without EnvFilePath set:\n%s", s)
	}
	if !strings.Contains(s, "traefik.http.routers.app1.entrypoints") {
		t.Errorf("missing entrypoints label:\n%s", s)
	}
}
