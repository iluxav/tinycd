package compose

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type OverrideInput struct {
	AppName      string
	PrimarySvc   string
	Domain       string
	Port         int
	EnvFilePath  string // absolute or relative-to-override path; empty if none
	NetworkName  string // e.g. "tcd-proxy"
	CertResolver string // e.g. "le" — empty disables TLS router
}

// RenderOverride writes apps/<app>/override.yml merging Traefik labels, network, and env_file
// into the repo's primary service.
func RenderOverride(in OverrideInput, outPath string) error {
	labels := map[string]string{
		"traefik.enable": "true",
		fmt.Sprintf("traefik.http.routers.%s.rule", in.AppName):                                fmt.Sprintf("Host(`%s.%s`)", in.AppName, in.Domain),
		fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port", in.AppName):          fmt.Sprintf("%d", in.Port),
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

	doc := map[string]any{
		"services": map[string]any{
			in.PrimarySvc: svc,
		},
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
