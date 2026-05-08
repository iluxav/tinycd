package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type OverrideInput struct {
	AppName      string
	PrimarySvc   string
	Domain       string
	Port         int
	EnvFilePath  string       // absolute or relative-to-override path; empty if none
	NetworkName  string       // e.g. "tcd-proxy"
	CertResolver string       // e.g. "le" — empty disables TLS router
	Aliases      []string     // additional hostnames to match, e.g. "hd.etunl.com"
	AutoVolumes  []AutoVolume // host-backed mounts derived from image VOLUME directives
}

// AutoVolume is a bind mount tcd attaches to <Service> at <MountPath>, backed
// by <HostPath> on the host. HostPath should be absolute.
type AutoVolume struct {
	Service   string
	HostPath  string
	MountPath string
}

// RenderOverride writes apps/<app>/override.yml merging Traefik labels, network, and env_file
// into the repo's primary service.
func RenderOverride(in OverrideInput, outPath string) error {
	hosts := append([]string{fmt.Sprintf("%s.%s", in.AppName, in.Domain)}, in.Aliases...)
	rule := buildHostRule(hosts)
	labels := map[string]string{
		"traefik.enable": "true",
		fmt.Sprintf("traefik.http.routers.%s.rule", in.AppName):                       rule,
		fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port", in.AppName): fmt.Sprintf("%d", in.Port),
	}
	if in.CertResolver != "" {
		labels[fmt.Sprintf("traefik.http.routers.%s.entrypoints", in.AppName)] = "websecure"
		labels[fmt.Sprintf("traefik.http.routers.%s.tls.certresolver", in.AppName)] = in.CertResolver
	} else {
		labels[fmt.Sprintf("traefik.http.routers.%s.entrypoints", in.AppName)] = "web"
	}

	svc := map[string]any{
		"networks": []string{in.NetworkName},
		"labels":   labels,
	}
	if in.EnvFilePath != "" {
		svc["env_file"] = []string{in.EnvFilePath}
	}

	services := map[string]any{
		in.PrimarySvc: svc,
	}

	// Group auto-volumes by service. The primary service gets its volumes
	// appended to the existing entry; other services get a fresh entry that
	// compose will merge with the user's own service definition.
	byService := map[string][]string{}
	for _, v := range in.AutoVolumes {
		byService[v.Service] = append(byService[v.Service], v.HostPath+":"+v.MountPath)
	}
	for svcName, mounts := range byService {
		if svcName == in.PrimarySvc {
			svc["volumes"] = mounts
			continue
		}
		services[svcName] = map[string]any{
			"volumes": mounts,
		}
	}

	doc := map[string]any{
		"services": services,
		"networks": map[string]any{
			in.NetworkName: map[string]any{
				"external": true,
			},
		},
	}

	data, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	header := []byte("# managed by tcd — do not edit\n")
	return os.WriteFile(outPath, append(header, data...), 0o644)
}

// buildHostRule produces a Traefik rule matching any of the given hostnames.
// One host → Host(`a`). Multiple → Host(`a`) || Host(`b`).
func buildHostRule(hosts []string) string {
	parts := make([]string, 0, len(hosts))
	for _, h := range hosts {
		if h == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("Host(`%s`)", h))
	}
	return strings.Join(parts, " || ")
}
