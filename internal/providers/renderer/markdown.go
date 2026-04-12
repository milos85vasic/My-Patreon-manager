package renderer

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"text/template"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type MarkdownRenderer struct{}

func NewMarkdownRenderer() *MarkdownRenderer { return &MarkdownRenderer{} }

func (r *MarkdownRenderer) Format() string { return "markdown" }

func (r *MarkdownRenderer) SupportedContentTypes() []string {
	return []string{"text/markdown", "text/x-markdown"}
}

func (r *MarkdownRenderer) Render(ctx context.Context, content models.Content, opts RenderOptions) ([]byte, error) {
	var sb strings.Builder

	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("title: %q\n", content.Title))
	if len(opts.TierMapping) > 0 {
		tiers := make([]string, 0, len(opts.TierMapping))
		for _, v := range opts.TierMapping {
			tiers = append(tiers, v)
		}
		sb.WriteString(fmt.Sprintf("tiers: %q\n", strings.Join(tiers, ",")))
	}
	sb.WriteString("generated: true\n")
	sb.WriteString("---\n\n")

	body := content.Body
	body = applyTemplateVariables(body, content)
	sb.WriteString(body)

	// Add mirror URLs section if any
	if len(opts.MirrorURLs) > 0 {
		sb.WriteString("\n\n## Get the Code\n\n")
		for _, mirror := range opts.MirrorURLs {
			sb.WriteString(fmt.Sprintf("- [%s](%s) – %s\n", mirror.Service, mirror.URL, mirror.Label))
		}
	}

	result := sb.String()
	result = lintMarkdown(result)
	return []byte(result), nil
}

func applyTemplateVariables(body string, content models.Content) string {
	tmpl, err := template.New("md").Option("missingkey=error").Funcs(SafeFuncs()).Parse(body)
	if err != nil {
		return body // fallback to raw on parse error
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, content); err != nil {
		return body // fallback to raw on execution error
	}
	return buf.String()
}

func lintMarkdown(content string) string {
	brokenLink := regexp.MustCompile(`\[([^\]]*)\]\(\s*\)`)
	content = brokenLink.ReplaceAllString(content, "[$1]")
	return content
}
