package sync

import (
	"fmt"
)

type ConflictType string

const (
	ConflictManualEdit ConflictType = "manual_edit"
	ConflictRename     ConflictType = "rename"
)

type Conflict struct {
	Type         ConflictType
	RepositoryID string
	Message      string
}

func DetectManualEditConflict(localHash, remoteHash string) (*Conflict, error) {
	if localHash == "" || remoteHash == "" {
		return nil, nil
	}
	if localHash != remoteHash {
		return &Conflict{
			Type:    ConflictManualEdit,
			Message: fmt.Sprintf("content hash mismatch: local=%s remote=%s", localHash, remoteHash),
		}, nil
	}
	return nil, nil
}

func DetectRenameError(repoName string, statusCode int) (*Conflict, error) {
	if statusCode == 404 {
		return &Conflict{
			Type:    ConflictRename,
			Message: fmt.Sprintf("repository %s returned 404, possible rename", repoName),
		}, nil
	}
	return nil, nil
}
