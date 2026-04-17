package image

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestImageRequestDefaults(t *testing.T) {
	req := ImageRequest{
		Prompt:       "a sunset over mountains",
		RepositoryID: "repo-1",
	}
	req.SetDefaults()
	assert.Equal(t, "1792x1024", req.Size)
	assert.Equal(t, "hd", req.Quality)
	assert.Equal(t, "png", req.Format)
}

func TestImageRequestDefaultsPreserveExisting(t *testing.T) {
	req := ImageRequest{
		Prompt:       "test",
		Size:         "1024x1024",
		Quality:      "standard",
		Format:       "jpeg",
		RepositoryID: "repo-1",
	}
	req.SetDefaults()
	assert.Equal(t, "1024x1024", req.Size)
	assert.Equal(t, "standard", req.Quality)
	assert.Equal(t, "jpeg", req.Format)
}

func TestImageResult_HasData(t *testing.T) {
	r := &ImageResult{Data: []byte{1, 2, 3}}
	assert.True(t, r.HasData())

	r2 := &ImageResult{URL: "https://example.com/img.png"}
	assert.True(t, r2.HasData())

	r3 := &ImageResult{}
	assert.False(t, r3.HasData())
}

type mockProvider struct {
	name      string
	available bool
	result    *ImageResult
	err       error
}

func (m *mockProvider) GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error) {
	return m.result, m.err
}

func (m *mockProvider) ProviderName() string {
	return m.name
}

func (m *mockProvider) IsAvailable(ctx context.Context) bool {
	return m.available
}

func TestMockProviderImplementsInterface(t *testing.T) {
	var _ ImageProvider = &mockProvider{}
	assert.True(t, true)
}
