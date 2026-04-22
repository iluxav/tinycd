package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// RootComposeInput builds the initial root compose.yml (traefik + empty include).
type RootComposeInput struct {
	ACMEEmail string // if empty, ACME/TLS blocks are omitted
}

func RenderRootCompose(in RootComposeInput) ([]byte, error) {
	traefikCmd := []string{
		"--providers.docker=true",
		"--providers.docker.exposedbydefault=false",
		"--entrypoints.web.address=:80",
	}
	volumes := []string{
		"/var/run/docker.sock:/var/run/docker.sock:ro",
	}
	if in.ACMEEmail != "" {
		traefikCmd = append(traefikCmd,
			"--entrypoints.websecure.address=:443",
			"--certificatesresolvers.le.acme.email="+in.ACMEEmail,
			"--certificatesresolvers.le.acme.storage=/acme.json",
			"--certificatesresolvers.le.acme.httpchallenge=true",
			"--certificatesresolvers.le.acme.httpchallenge.entrypoint=web",
		)
		volumes = append(volumes, "./traefik/acme.json:/acme.json")
	}

	ports := []string{"80:80"}
	if in.ACMEEmail != "" {
		ports = append(ports, "443:443")
	}

	doc := map[string]any{
		"services": map[string]any{
			"traefik": map[string]any{
				"image":    "traefik:v3",
				"restart":  "unless-stopped",
				"command":  traefikCmd,
				"ports":    ports,
				"networks": []string{"tcd-proxy"},
				"volumes":  volumes,
			},
		},
		"networks": map[string]any{
			"tcd-proxy": map[string]any{
				"name": "tcd-proxy",
			},
		},
		"include": []any{}, // populated by tcd deploy
	}

	data, err := yaml.Marshal(doc)
	if err != nil {
		return nil, err
	}
	header := []byte("# managed by tcd — `include:` entries are added/removed automatically\n")
	return append(header, data...), nil
}

// IncludeEntry represents one app's include entry:
// - path: [apps/<app>/repo/compose.yml, apps/<app>/override.yml]
type IncludeEntry struct {
	AppName string
	Paths   []string
}

// AddInclude upserts an include entry for appName with the given paths.
// Idempotent: updates paths if appName already present.
func AddInclude(rootPath, appName string, paths []string) error {
	entries, rest, err := readRoot(rootPath)
	if err != nil {
		return err
	}
	found := false
	for i, e := range entries {
		if e.AppName == appName {
			entries[i].Paths = paths
			found = true
			break
		}
	}
	if !found {
		entries = append(entries, IncludeEntry{AppName: appName, Paths: paths})
	}
	return writeRoot(rootPath, entries, rest)
}

// RemoveInclude removes the entry for appName. Idempotent: no-op if missing.
func RemoveInclude(rootPath, appName string) error {
	entries, rest, err := readRoot(rootPath)
	if err != nil {
		return err
	}
	filtered := entries[:0]
	for _, e := range entries {
		if e.AppName != appName {
			filtered = append(filtered, e)
		}
	}
	return writeRoot(rootPath, filtered, rest)
}

// ListIncludes returns app names currently referenced.
func ListIncludes(rootPath string) ([]string, error) {
	entries, _, err := readRoot(rootPath)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.AppName)
	}
	sort.Strings(names)
	return names, nil
}

// readRoot parses compose.yml, extracting our include entries and the rest of the document.
func readRoot(path string) ([]IncludeEntry, map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if doc == nil {
		doc = map[string]any{}
	}
	raw, _ := doc["include"].([]any)
	var entries []IncludeEntry
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		paths := extractPathList(m["path"])
		if len(paths) == 0 {
			continue
		}
		entries = append(entries, IncludeEntry{AppName: appFromPaths(paths), Paths: paths})
	}
	delete(doc, "include")
	return entries, doc, nil
}

func writeRoot(path string, entries []IncludeEntry, rest map[string]any) error {
	includes := make([]any, 0, len(entries))
	for _, e := range entries {
		includes = append(includes, map[string]any{
			"path": stringsToAny(e.Paths),
		})
	}
	rest["include"] = includes
	data, err := yaml.Marshal(rest)
	if err != nil {
		return err
	}
	header := []byte("# managed by tcd — `include:` entries are added/removed automatically\n")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(header, data...), 0o644)
}

func extractPathList(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func stringsToAny(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}

// appFromPaths infers the app name from a path like "apps/<app>/repo/compose.yml".
func appFromPaths(paths []string) string {
	for _, p := range paths {
		parts := splitPath(p)
		for i, part := range parts {
			if part == "apps" && i+1 < len(parts) {
				return parts[i+1]
			}
		}
	}
	return ""
}

func splitPath(p string) []string {
	var out []string
	cur := ""
	for _, r := range p {
		if r == '/' || r == '\\' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
