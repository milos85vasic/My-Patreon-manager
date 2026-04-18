package models

import "time"

// UnmatchedPatreonPost represents a Patreon post pulled during first-run
// import that couldn't be mapped to a local repository. Operators resolve
// these manually by linking each post to a repository; the resolved_at /
// resolved_repository_id columns record that action.
type UnmatchedPatreonPost struct {
	ID                   string
	PatreonPostID        string
	Title                string
	URL                  string
	PublishedAt          *time.Time
	RawPayload           string
	DiscoveredAt         time.Time
	ResolvedRepositoryID *string
	ResolvedAt           *time.Time
}
