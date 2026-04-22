package compose

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeRootFixture(t *testing.T, dir string) string {
	t.Helper()
	data, err := RenderRootCompose(RootComposeInput{ACMEEmail: "me@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "compose.yml")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRootCompose_AddListRemove(t *testing.T) {
	dir := t.TempDir()
	p := writeRootFixture(t, dir)

	// Add app1
	if err := AddInclude(p, "app1", []string{"apps/app1/repo/compose.yml", "apps/app1/override.yml"}); err != nil {
		t.Fatal(err)
	}
	// Add app2
	if err := AddInclude(p, "app2", []string{"apps/app2/repo/compose.yml", "apps/app2/override.yml"}); err != nil {
		t.Fatal(err)
	}
	// Re-adding app1 should be idempotent / replace.
	if err := AddInclude(p, "app1", []string{"apps/app1/repo/compose.yml", "apps/app1/override.yml"}); err != nil {
		t.Fatal(err)
	}

	names, err := ListIncludes(p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, []string{"app1", "app2"}) {
		t.Errorf("names = %v, want [app1 app2]", names)
	}

	if err := RemoveInclude(p, "app1"); err != nil {
		t.Fatal(err)
	}
	names, _ = ListIncludes(p)
	if !reflect.DeepEqual(names, []string{"app2"}) {
		t.Errorf("after rm: names = %v, want [app2]", names)
	}

	// Remove missing: no-op.
	if err := RemoveInclude(p, "not-there"); err != nil {
		t.Errorf("remove missing should be no-op, got %v", err)
	}
}

func TestRootCompose_TraefikPreserved(t *testing.T) {
	dir := t.TempDir()
	p := writeRootFixture(t, dir)
	_ = AddInclude(p, "app1", []string{"apps/app1/repo/compose.yml", "apps/app1/override.yml"})
	data, _ := os.ReadFile(p)
	s := string(data)
	for _, want := range []string{"traefik", "tcd-proxy", "include"} {
		if !contains(s, want) {
			t.Errorf("missing %q in root compose:\n%s", want, s)
		}
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
