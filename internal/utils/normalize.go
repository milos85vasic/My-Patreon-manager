package utils

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	httpsPattern = regexp.MustCompile(`^https?://([^/]+)/([^/]+)/([^/]+?)(?:\.git)?$`)
	sshPattern   = regexp.MustCompile(`^ssh://git@([^/]+)/([^/]+)/([^/]+?)(?:\.git)?$`)
	scpPattern   = regexp.MustCompile(`^git@([^:]+):([^/]+)/([^/]+?)(?:\.git)?$`)
)

func NormalizeToSSH(url string) string {
	url = strings.TrimSpace(url)
	if strings.HasPrefix(url, "git@") && strings.Contains(url, ":") {
		if strings.HasPrefix(url, "git@") && !strings.HasPrefix(url, "ssh://") {
			parts := strings.SplitN(url, ":", 2)
			if len(parts) == 2 && strings.Contains(parts[1], "/") {
				return url
			}
		}
	}

	if m := scpPattern.FindStringSubmatch(url); len(m) == 4 {
		return fmt.Sprintf("git@%s:%s/%s.git", m[1], m[2], m[3])
	}

	if m := httpsPattern.FindStringSubmatch(url); len(m) == 4 {
		return fmt.Sprintf("git@%s:%s/%s.git", m[1], m[2], m[3])
	}

	if m := sshPattern.FindStringSubmatch(url); len(m) == 4 {
		return fmt.Sprintf("git@%s:%s/%s.git", m[1], m[2], m[3])
	}

	return url
}

func NormalizeHTTPS(url string) string {
	url = strings.TrimSpace(url)
	if m := scpPattern.FindStringSubmatch(url); len(m) == 4 {
		return fmt.Sprintf("https://%s/%s/%s.git", m[1], m[2], m[3])
	}
	if m := sshPattern.FindStringSubmatch(url); len(m) == 4 {
		return fmt.Sprintf("https://%s/%s/%s.git", m[1], m[2], m[3])
	}
	return url
}
