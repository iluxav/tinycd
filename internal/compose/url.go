package compose

import (
	"fmt"
	"path"
	"strings"
)

// NormalizeRepoURL accepts "user/repo", "git@host:user/repo.git", "https://host/user/repo"
// and returns an SSH-style URL plus the derived app name.
func NormalizeRepoURL(input string) (url, name string, err error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", "", fmt.Errorf("empty repo")
	}

	switch {
	case strings.HasPrefix(s, "git@"):
		// git@host:user/repo(.git)
		url = s
		colon := strings.IndexByte(s, ':')
		if colon < 0 {
			return "", "", fmt.Errorf("invalid SSH URL: %s", s)
		}
		name = baseName(s[colon+1:])
	case strings.HasPrefix(s, "ssh://"):
		url = s
		name = baseName(s)
	case strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://"):
		// https://host/user/repo(.git) → git@host:user/repo.git
		rest := strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://")
		slash := strings.IndexByte(rest, '/')
		if slash < 0 {
			return "", "", fmt.Errorf("invalid HTTP URL: %s", s)
		}
		host, pathPart := rest[:slash], rest[slash+1:]
		pathPart = strings.TrimSuffix(pathPart, "/")
		if !strings.HasSuffix(pathPart, ".git") {
			pathPart += ".git"
		}
		url = "git@" + host + ":" + pathPart
		name = baseName(pathPart)
	default:
		// shorthand: user/repo
		if !strings.Contains(s, "/") {
			return "", "", fmt.Errorf("invalid repo shorthand (expected user/repo): %s", s)
		}
		shorthand := s
		if !strings.HasSuffix(shorthand, ".git") {
			shorthand += ".git"
		}
		url = "git@github.com:" + shorthand
		name = baseName(shorthand)
	}
	if name == "" {
		return "", "", fmt.Errorf("could not derive app name from %s", input)
	}
	return url, name, nil
}

func baseName(p string) string {
	p = strings.TrimSuffix(p, ".git")
	return path.Base(p)
}
