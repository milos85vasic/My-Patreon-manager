package models

import "time"

// ContentRevision is one row of the content_revisions table. Revisions
// are immutable: only the `Status` plus the published-marker columns
// (PatreonPostID, PublishedToPatreonAt) may be UPDATEd after insert, and
// only via forward-only transitions enforced at the store layer.
type ContentRevision struct {
	ID                   string
	RepositoryID         string
	Version              int
	Source               string // "patreon_import" | "generated" | "manual_edit"
	Status               string // "pending_review" | "approved" | "rejected" | "superseded"
	Title                string
	Body                 string
	Fingerprint          string
	IllustrationID       *string
	GeneratorVersion     string
	SourceCommitSHA      string
	PatreonPostID        *string
	PublishedToPatreonAt *time.Time
	EditedFromRevisionID *string
	Author               string
	CreatedAt            time.Time
}

// Exported revision status constants. Using these rather than duplicating
// the literals keeps the store layer and the status graph in sync.
const (
	RevisionStatusPendingReview = "pending_review"
	RevisionStatusApproved      = "approved"
	RevisionStatusRejected      = "rejected"
	RevisionStatusSuperseded    = "superseded"
)

// contentRevisionStatusGraph lists the legal forward-only transitions of
// the Status field. Any transition not listed here is rejected at the
// store layer. State machine documented in
// docs/superpowers/specs/2026-04-18-process-command-design.md §State Machines.
var contentRevisionStatusGraph = map[string]map[string]bool{
	RevisionStatusPendingReview: {RevisionStatusApproved: true, RevisionStatusRejected: true, RevisionStatusSuperseded: true},
	RevisionStatusApproved:      {RevisionStatusSuperseded: true},
	RevisionStatusRejected:      {},
	RevisionStatusSuperseded:    {},
}

// IsLegalRevisionStatusTransition reports whether `to` is a legal next state
// from `from` under the revision status graph.
func IsLegalRevisionStatusTransition(from, to string) bool {
	if next, ok := contentRevisionStatusGraph[from]; ok {
		return next[to]
	}
	return false
}
