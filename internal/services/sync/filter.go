package sync

import (
	"strings"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

func ApplyFilter(repos []models.Repository, f SyncFilter, stateFn func(string) (*models.SyncState, error)) []models.Repository {
	if f.Org == "" && f.RepoURL == "" && f.Pattern == "" && f.Since == "" && !f.ChangedOnly {
		return repos
	}

	var filtered []models.Repository
	for _, r := range repos {
		if f.Org != "" && r.Owner != f.Org {
			continue
		}
		if f.RepoURL != "" {
			if r.URL != f.RepoURL && r.HTTPSURL != f.RepoURL {
				continue
			}
		}
		if f.Pattern != "" {
			matched, _ := matchGlob(r.Name, f.Pattern)
			if !matched {
				continue
			}
		}
		if f.Since != "" {
			since, err := time.Parse(time.RFC3339, f.Since)
			if err == nil && r.UpdatedAt.Before(since) {
				continue
			}
		}
		if f.ChangedOnly && stateFn != nil {
			state, err := stateFn(r.ID)
			if err == nil && state != nil {
				if state.LastContentHash != "" {
					continue
				}
			}
		}
		filtered = append(filtered, r)
	}
	return filtered
}

func matchGlob(name, pattern string) (bool, error) {
	if pattern == "*" {
		return true, nil
	}
	if strings.Contains(pattern, "*") {
		parts := strings.SplitN(pattern, "*", 2)
		if len(parts) == 2 {
			return strings.HasPrefix(name, parts[0]) && strings.HasSuffix(name, parts[1]), nil
		}
		return strings.HasPrefix(name, parts[0]), nil
	}
	return name == pattern, nil
}
