package renderer

import (
	"strings"
	"text/template"
	"time"
)

// SafeFuncs returns a vetted set of template functions safe for use in
// user-facing Markdown templates.  No filesystem, exec, or network
// access is exposed.
func SafeFuncs() template.FuncMap {
	return template.FuncMap{
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"trim":  strings.TrimSpace,
		"short": func(s string) string {
			if len(s) > 7 {
				return s[:7]
			}
			return s
		},
		"now":      func() time.Time { return time.Now().UTC() },
		"date":     func(t time.Time) string { return t.Format("2006-01-02") },
		"join":     strings.Join,
		"replace":  strings.ReplaceAll,
		"contains": strings.Contains,
		"default": func(d, v string) string {
			if v == "" {
				return d
			}
			return v
		},
	}
}
