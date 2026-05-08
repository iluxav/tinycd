package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/iluxav/tinycd/internal/compose"
	"github.com/iluxav/tinycd/internal/config"
)

// imageInspector is the slice of dc.Client we depend on. Defined as an
// interface so the filter logic is testable without docker.
type imageInspector interface {
	ServiceImages() (map[string]string, error)
	InspectImage(image string) (volumes []string, user string, err error)
	ResolveUser(image, user string) (uid, gid int, err error)
}

// collectAutoVolumes walks each service in the parsed compose, inspects its
// image for VOLUME declarations, filters out paths the user has already mapped,
// resolves the runtime UID/GID, prepares the host-side directory (mkdir +
// chown), and returns both the override input and the persisted state slice.
func collectAutoVolumes(cfg *config.Config, appName string, parsed *compose.ParsedCompose, client imageInspector) ([]compose.AutoVolume, []config.VolumeMount, error) {
	images, err := client.ServiceImages()
	if err != nil {
		return nil, nil, err
	}

	appDir := cfg.AppDir(appName)
	var (
		auto   []compose.AutoVolume
		mounts []config.VolumeMount
	)

	// Iterate services in declaration order for deterministic output.
	for _, svcName := range parsed.Order() {
		image, ok := images[svcName]
		if !ok || image == "" {
			continue
		}
		volPaths, user, err := client.InspectImage(image)
		if err != nil {
			return nil, nil, err
		}
		if len(volPaths) == 0 {
			continue
		}

		existing := compose.ExistingVolumeTargets(parsed.Services[svcName])

		// Filter out user-mapped paths, then sort for stable output.
		filtered := filterAutoVolumePaths(volPaths, existing)
		if len(filtered) == 0 {
			continue
		}

		uid, gid, err := client.ResolveUser(image, user)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: resolve user for %s: %v (defaulting to root)\n", image, err)
			uid, gid = 0, 0
		}

		for _, mountPath := range filtered {
			hostPath := filepath.Join(appDir, "volumes", svcName, mountPath)
			if err := os.MkdirAll(hostPath, 0o755); err != nil {
				return nil, nil, fmt.Errorf("mkdir %s: %w", hostPath, err)
			}
			if err := os.Chown(hostPath, uid, gid); err != nil {
				fmt.Fprintf(os.Stderr, "warn: chown %s to %d:%d: %v\n", hostPath, uid, gid, err)
			}
			auto = append(auto, compose.AutoVolume{
				Service:   svcName,
				HostPath:  hostPath,
				MountPath: mountPath,
			})
			mounts = append(mounts, config.VolumeMount{
				Service:   svcName,
				HostPath:  hostPath,
				MountPath: mountPath,
			})
		}
	}
	return auto, mounts, nil
}

// filterAutoVolumePaths returns the subset of declared VOLUME paths that the
// user has not already mapped, sorted for determinism.
func filterAutoVolumePaths(declared []string, existing map[string]bool) []string {
	out := make([]string, 0, len(declared))
	for _, p := range declared {
		if !existing[p] {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}
