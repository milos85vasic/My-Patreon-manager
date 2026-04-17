package illustration

import (
	"fmt"
	"strings"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type PromptBuilder struct {
	defaultStyle string
}

func NewPromptBuilder(defaultStyle string) *PromptBuilder {
	return &PromptBuilder{defaultStyle: defaultStyle}
}

func (pb *PromptBuilder) Build(repo *models.Repository, content *models.GeneratedContent) string {
	var repoName, repoDesc, repoLang string
	var repoTopics []string
	var contentTitle string
	if repo != nil {
		repoName = repo.Name
		repoDesc = repo.Description
		repoLang = repo.PrimaryLanguage
		repoTopics = repo.Topics
	}
	if content != nil {
		contentTitle = content.Title
	}
	return pb.BuildFromFields(repoName, repoDesc, repoLang, repoTopics, contentTitle, "")
}

func (pb *PromptBuilder) BuildFromFields(repoName, repoDesc, repoLang string, repoTopics []string, contentTitle, _ string) string {
	parts := []string{}

	if contentTitle != "" {
		parts = append(parts, fmt.Sprintf("Illustration for article \"%s\"", contentTitle))
	}

	if repoName != "" {
		parts = append(parts, fmt.Sprintf("about the %s project", repoName))
	}

	if repoLang != "" {
		parts = append(parts, fmt.Sprintf("using %s", repoLang))
	}

	if len(repoTopics) > 0 {
		parts = append(parts, fmt.Sprintf("topics: %s", strings.Join(repoTopics, ", ")))
	}

	if repoDesc != "" && len(repoDesc) < 200 {
		parts = append(parts, repoDesc)
	}

	if pb.defaultStyle != "" {
		parts = append(parts, pb.defaultStyle)
	}

	return strings.Join(parts, ". ")
}
