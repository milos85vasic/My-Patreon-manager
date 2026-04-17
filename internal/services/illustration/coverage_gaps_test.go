package illustration

import (
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

func TestPromptBuilder_CoverageGaps(t *testing.T) {
	t.Run("PromptBuilderWithEmptyContent", func(t *testing.T) {
		pb := &PromptBuilder{defaultStyle: "default"}
		repo := &models.Repository{Name: "test"}
		content := &models.GeneratedContent{Title: "", Body: ""}
		result := pb.Build(repo, content)
		if result == "" {
			t.Error("expected non-empty prompt")
		}
	})

	t.Run("PromptBuilderWithAllFields", func(t *testing.T) {
		pb := &PromptBuilder{defaultStyle: "modern style"}
		repo := &models.Repository{
			Name:            "my-project",
			Description:     "A great project",
			PrimaryLanguage: "Go",
			Topics:          []string{"api", "rest"},
		}
		content := &models.GeneratedContent{
			Title: "My API",
			Body:  "Content here",
		}
		result := pb.Build(repo, content)
		if result == "" {
			t.Error("expected non-empty prompt")
		}
	})

	t.Run("PromptBuilderBuildFromFields", func(t *testing.T) {
		pb := &PromptBuilder{defaultStyle: "modern style"}
		result := pb.BuildFromFields("repo", "desc", "Go", []string{"topic"}, "title", "body")
		if result == "" {
			t.Error("expected non-empty prompt")
		}
	})

	t.Run("PromptBuilderBuildFromFieldsWithTopics", func(t *testing.T) {
		pb := &PromptBuilder{defaultStyle: "modern style"}
		result := pb.BuildFromFields("repo", "desc", "Go", []string{"api", "rest", "web"}, "title", "body")
		if result == "" {
			t.Error("expected non-empty prompt")
		}
	})
}

func TestStyleLoader_CoverageGaps(t *testing.T) {
	t.Run("StyleLoaderWithRepoOverride", func(t *testing.T) {
		sl := NewStyleLoader("global style")
		override := "override style"
		result := sl.LoadStyle(&override)
		if result != override {
			t.Errorf("expected %s, got %s", override, result)
		}
	})

	t.Run("StyleLoaderNilOverride", func(t *testing.T) {
		sl := NewStyleLoader("global style")
		result := sl.LoadStyle(nil)
		if result != "global style" {
			t.Errorf("expected global style, got %s", result)
		}
	})
}
