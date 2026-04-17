package illustration

import (
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestPromptBuilder_Build(t *testing.T) {
	repo := &models.Repository{
		Name:            "my-go-project",
		Description:     "A scalable API built with Go",
		PrimaryLanguage: "Go",
		Topics:          []string{"api", "microservices"},
	}
	content := &models.GeneratedContent{
		Title: "Building Scalable APIs",
	}

	pb := NewPromptBuilder("modern tech illustration, clean lines")
	prompt := pb.Build(repo, content)
	assert.Contains(t, prompt, "my-go-project")
	assert.Contains(t, prompt, "Go")
	assert.Contains(t, prompt, "modern tech illustration")
}

func TestPromptBuilder_BuildFromFields(t *testing.T) {
	pb := NewPromptBuilder("default style")
	prompt := pb.BuildFromFields("repo-name", "A description", "Rust", []string{"web", "cli"}, "Article Title", "")
	assert.Contains(t, prompt, "repo-name")
	assert.Contains(t, prompt, "Rust")
	assert.Contains(t, prompt, "web, cli")
	assert.Contains(t, prompt, "default style")
}

func TestPromptBuilder_EmptyFields(t *testing.T) {
	pb := NewPromptBuilder("")
	prompt := pb.BuildFromFields("", "", "", nil, "", "")
	assert.Empty(t, prompt)
}

func TestStyleLoader_DefaultStyle(t *testing.T) {
	sl := NewStyleLoader("global default style")
	style := sl.LoadStyle(nil)
	assert.Equal(t, "global default style", style)
}

func TestStyleLoader_RepoOverride(t *testing.T) {
	sl := NewStyleLoader("global default")
	override := "custom repo style"
	style := sl.LoadStyle(&override)
	assert.Equal(t, "custom repo style", style)
}

func TestStyleLoader_EmptyOverride(t *testing.T) {
	sl := NewStyleLoader("global default")
	empty := ""
	style := sl.LoadStyle(&empty)
	assert.Equal(t, "global default", style)
}
