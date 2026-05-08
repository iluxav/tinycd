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
			if err := ensureContainerWritable(hostPath, uid, gid); err != nil {
				fmt.Fprintf(os.Stderr, "warn: %s\n", err)
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

// ensureContainerWritable makes hostPath writable from inside the container,
// regardless of whether tcd itself runs as root.
//
//   - When tcd is root (the common systemd-deployed case): chown to the image's
//     runtime UID:GID. Container process owns its data dir, no extra perms.
//   - When tcd is non-root (e.g. user ran `tcd deploy` on macOS or a personal
//     linux box): non-root cannot chown to a foreign UID, so fall back to
//     chmod 0o777. The container user can now write; the cost is that any
//     local user on the host can also read/write the data, which is fine on a
//     single-user box and the only option short of running tcd as root.
func ensureContainerWritable(hostPath string, uid, gid int) error {
	if os.Geteuid() == 0 {
		if err := os.Chown(hostPath, uid, gid); err != nil {
			return fmt.Errorf("chown %s to %d:%d: %w", hostPath, uid, gid, err)
		}
		return nil
	}
	if err := os.Chmod(hostPath, 0o777); err != nil {
		return fmt.Errorf("chmod %s 0o777: %w", hostPath, err)
	}
	return nil
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
