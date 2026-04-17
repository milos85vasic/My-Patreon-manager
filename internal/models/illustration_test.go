package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIllustrationIDGeneration(t *testing.T) {
	ill := &Illustration{
		GeneratedContentID: "content-123",
		RepositoryID:       "repo-456",
		Prompt:             "test prompt",
		Style:              "default style",
	}
	id := ill.GenerateID()
	assert.NotEmpty(t, id)
	assert.Contains(t, id, "ill_")
	assert.Equal(t, id, ill.ID)
}

func TestIllustrationIDConsistency(t *testing.T) {
	ill1 := &Illustration{GeneratedContentID: "c1", RepositoryID: "r1"}
	ill2 := &Illustration{GeneratedContentID: "c1", RepositoryID: "r1"}
	assert.Equal(t, ill1.GenerateID(), ill2.GenerateID())

	ill3 := &Illustration{GeneratedContentID: "c2", RepositoryID: "r1"}
	assert.NotEqual(t, ill1.GenerateID(), ill3.GenerateID())
}

func TestIllustrationFingerprint(t *testing.T) {
	ill := &Illustration{
		Prompt: "a beautiful landscape",
		Style:  "watercolor",
	}
	fp := ill.ComputeFingerprint()
	assert.NotEmpty(t, fp)

	ill2 := &Illustration{
		Prompt: "a beautiful landscape",
		Style:  "watercolor",
	}
	assert.Equal(t, fp, ill2.ComputeFingerprint())

	ill3 := &Illustration{
		Prompt: "a different prompt",
		Style:  "watercolor",
	}
	assert.NotEqual(t, fp, ill3.ComputeFingerprint())
}

func TestIllustrationDefaultValues(t *testing.T) {
	ill := &Illustration{}
	ill.SetDefaults()
	assert.Equal(t, "png", ill.Format)
	assert.Equal(t, "1792x1024", ill.Size)
	assert.NotEmpty(t, ill.ID)
	assert.False(t, ill.CreatedAt.IsZero())
}

func TestIllustrationSetDefaultsPreservesExisting(t *testing.T) {
	now := time.Now()
	ill := &Illustration{
		ID:        "custom-id",
		Format:    "jpeg",
		Size:      "1024x1024",
		CreatedAt: now,
	}
	ill.SetDefaults()
	assert.Equal(t, "custom-id", ill.ID)
	assert.Equal(t, "jpeg", ill.Format)
	assert.Equal(t, "1024x1024", ill.Size)
	assert.Equal(t, now, ill.CreatedAt)
}
