package renderer

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type HTMLRenderer struct{}

func NewHTMLRenderer() *HTMLRenderer { return &HTMLRenderer{} }

func (r *HTMLRenderer) Format() string { return "html" }

func (r *HTMLRenderer) SupportedContentTypes() []string {
	return []string{"text/html"}
}

func (r *HTMLRenderer) Render(ctx context.Context, content models.Content, opts RenderOptions) ([]byte, error) {
	var sb strings.Builder

	sb.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	sb.WriteString("<meta charset=\"UTF-8\">\n")
	sb.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n")
	sb.WriteString(fmt.Sprintf("<title>%s</title>\n", html.EscapeString(content.Title)))
	sb.WriteString("<style>\n")
	sb.WriteString("body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 800px; margin: 0 auto; padding: 2rem; line-height: 1.6; }\n")
	sb.WriteString("img { max-width: 100%; height: auto; }\n")
	sb.WriteString("details { margin: 1rem 0; }\n")
	sb.WriteString("summary { cursor: pointer; font-weight: bold; }\n")
	sb.WriteString("@media print { body { max-width: 100%; padding: 0; } }\n")
	sb.WriteString("</style>\n")
	sb.WriteString("</head>\n<body>\n")

	body := markdownToHTML(content.Body)
	body = sanitizeScripts(body)
	sb.WriteString(body)

	sb.WriteString("\n</body>\n</html>")
	return []byte(sb.String()), nil
}

func markdownToHTML(md string) string {
	md = regexp.MustCompile(`(?m)^### (.+)$`).ReplaceAllString(md, "<h3>$1</h3>")
	md = regexp.MustCompile(`(?m)^## (.+)$`).ReplaceAllString(md, "<h2>$1</h2>")
	md = regexp.MustCompile(`(?m)^# (.+)$`).ReplaceAllString(md, "<h1>$1</h1>")
	md = regexp.MustCompile(`\*\*(.+?)\*\*`).ReplaceAllString(md, "<strong>$1</strong>")
	md = regexp.MustCompile(`\*(.+?)\*`).ReplaceAllString(md, "<em>$1</em>")
	md = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`).ReplaceAllString(md, "<a href=\"$2\">$1</a>")
	md = regexp.MustCompile("`([^`]+)`").ReplaceAllString(md, "<code>$1</code>")

	lines := strings.Split(md, "\n")
	var result strings.Builder
	inList := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			if !inList {
				result.WriteString("<ul>\n")
				inList = true
			}
			item := strings.TrimPrefix(trimmed, "- ")
			item = strings.TrimPrefix(item, "* ")
			result.WriteString(fmt.Sprintf("<li>%s</li>\n", item))
		} else {
			if inList {
				result.WriteString("</ul>\n")
				inList = false
			}
			if trimmed == "" {
				result.WriteString("\n")
			} else if !strings.HasPrefix(trimmed, "<") {
				result.WriteString(fmt.Sprintf("<p>%s</p>\n", trimmed))
			} else {
				result.WriteString(trimmed + "\n")
			}
		}
	}
	if inList {
		result.WriteString("</ul>\n")
	}
	return result.String()
}

func sanitizeScripts(htmlContent string) string {
	scriptTag := regexp.MustCompile(`(?i)<script[^>]*>.*?</script>`)
	htmlContent = scriptTag.ReplaceAllString(htmlContent, "")
	eventHandler := regexp.MustCompile(`(?i)\s*on\w+\s*=\s*["'][^"']*["']`)
	htmlContent = eventHandler.ReplaceAllString(htmlContent, "")
	return htmlContent
}
