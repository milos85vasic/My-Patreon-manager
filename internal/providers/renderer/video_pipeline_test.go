package renderer

import (
	"context"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestNewVideoPipeline(t *testing.T) {
	p := NewVideoPipeline(true)
	assert.True(t, p.IsEnabled())
	p2 := NewVideoPipeline(false)
	assert.False(t, p2.IsEnabled())
}

func TestVideoPipeline_IsEnabled(t *testing.T) {
	p := NewVideoPipeline(true)
	assert.True(t, p.IsEnabled())
	p = NewVideoPipeline(false)
	assert.False(t, p.IsEnabled())
}

func TestVideoPipeline_Render_Disabled(t *testing.T) {
	p := NewVideoPipeline(false)
	content := models.Content{Title: "Test", Body: "Body"}
	opts := RenderOptions{}
	result, err := p.Render(context.Background(), content, opts)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "video generation is not enabled")
}

func TestVideoPipeline_Render_Enabled(t *testing.T) {
	// Temporarily set PATH empty to ensure ffmpeg not found
	t.Setenv("PATH", "")
	p := NewVideoPipeline(true)
	content := models.Content{Title: "Test", Body: "Body"}
	opts := RenderOptions{}
	result, err := p.Render(context.Background(), content, opts)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "ffmpeg not found")
}

func TestVideoPipeline_CheckDiskSpace(t *testing.T) {
	p := NewVideoPipeline(true)
	err := p.CheckDiskSpace(100)
	assert.NoError(t, err)
}
