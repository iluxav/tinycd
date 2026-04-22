package compose

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseAndResolvePrimary_FirstService(t *testing.T) {
	yml := []byte(`
services:
  api:
    image: foo
  worker:
    image: bar
`)
	p, err := ParseCompose(yml)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ResolvePrimaryService(p, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "api" {
		t.Errorf("primary = %q, want %q", got, "api")
	}
}

func TestResolvePrimary_ExplicitOverride(t *testing.T) {
	yml := []byte(`
services:
  api: {image: foo}
  worker: {image: bar}
`)
	p, _ := ParseCompose(yml)
	got, err := ResolvePrimaryService(p, "worker")
	if err != nil {
		t.Fatal(err)
	}
	if got != "worker" {
		t.Errorf("primary = %q, want %q", got, "worker")
	}
}

func TestResolvePrimary_OverrideMissing(t *testing.T) {
	yml := []byte(`services: {api: {image: foo}}`)
	p, _ := ParseCompose(yml)
	_, err := ResolvePrimaryService(p, "nope")
	if err == nil {
		t.Fatal("expected error when override service absent")
	}
}

func TestResolvePrimary_LabelMarker_MapForm(t *testing.T) {
	yml := []byte(`
services:
  api:
    image: foo
  web:
    image: bar
    labels:
      tcd.primary: "true"
`)
	p, _ := ParseCompose(yml)
	got, err := ResolvePrimaryService(p, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "web" {
		t.Errorf("primary = %q, want %q", got, "web")
	}
}

func TestResolvePrimary_LabelMarker_ListForm(t *testing.T) {
	yml := []byte(`
services:
  api:
    image: foo
  web:
    image: bar
    labels:
      - "tcd.primary=true"
`)
	p, _ := ParseCompose(yml)
	got, err := ResolvePrimaryService(p, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "web" {
		t.Errorf("primary = %q, want %q", got, "web")
	}
}

func TestResolvePrimary_LabelBeatsOverride(t *testing.T) {
	yml := []byte(`
services:
  api:
    image: foo
    labels: {tcd.primary: "true"}
  web:
    image: bar
`)
	p, _ := ParseCompose(yml)
	// Even with override=web, label-marked api wins per design.
	got, _ := ResolvePrimaryService(p, "web")
	if got != "api" {
		t.Errorf("primary = %q, want %q", got, "api")
	}
}

func TestDetectComposeFile(t *testing.T) {
	dir := t.TempDir()
	if _, ok := DetectComposeFile(dir); ok {
		t.Fatal("expected no compose file in empty dir")
	}
	p := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(p, []byte("services: {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok := DetectComposeFile(dir)
	if !ok || got != p {
		t.Errorf("got (%q,%v), want (%q,true)", got, ok, p)
	}
}
