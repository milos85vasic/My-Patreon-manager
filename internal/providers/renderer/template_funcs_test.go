package renderer

import (
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestApplyTemplateVariables_Simple(t *testing.T) {
	content := models.Content{Title: "World", Body: "Hello {{ .Title }}!"}
	got := applyTemplateVariables(content.Body, content)
	assert.Equal(t, "Hello World!", got)
}

func TestApplyTemplateVariables_ShortFunc(t *testing.T) {
	content := models.Content{Title: "deadbeefcafe", Body: "{{ .Title | short }}"}
	got := applyTemplateVariables(content.Body, content)
	assert.Equal(t, "deadbee", got)
}

func TestApplyTemplateVariables_ShortFuncShortInput(t *testing.T) {
	content := models.Content{Title: "abc", Body: "{{ .Title | short }}"}
	got := applyTemplateVariables(content.Body, content)
	assert.Equal(t, "abc", got)
}

func TestApplyTemplateVariables_UpperFunc(t *testing.T) {
	content := models.Content{Title: "hello", Body: "{{ .Title | upper }}"}
	got := applyTemplateVariables(content.Body, content)
	assert.Equal(t, "HELLO", got)
}

func TestApplyTemplateVariables_LowerFunc(t *testing.T) {
	content := models.Content{Title: "HELLO", Body: "{{ .Title | lower }}"}
	got := applyTemplateVariables(content.Body, content)
	assert.Equal(t, "hello", got)
}

func TestApplyTemplateVariables_TrimFunc(t *testing.T) {
	content := models.Content{Title: "  hello  ", Body: "{{ .Title | trim }}"}
	got := applyTemplateVariables(content.Body, content)
	assert.Equal(t, "hello", got)
}

func TestApplyTemplateVariables_NoVars(t *testing.T) {
	content := models.Content{Body: "plain text"}
	got := applyTemplateVariables(content.Body, content)
	assert.Equal(t, "plain text", got)
}

func TestApplyTemplateVariables_MissingKeyFallback(t *testing.T) {
	content := models.Content{Body: "{{ .NonExistent }}"}
	got := applyTemplateVariables(content.Body, content)
	// missingkey=error causes Execute to fail; falls back to raw body
	assert.Equal(t, "{{ .NonExistent }}", got)
}

func TestApplyTemplateVariables_ParseErrorFallback(t *testing.T) {
	content := models.Content{Body: `{{ exec "ls" }}`}
	got := applyTemplateVariables(content.Body, content)
	// exec is not in SafeFuncs so parse fails; raw body returned
	assert.Equal(t, `{{ exec "ls" }}`, got)
}

func TestApplyTemplateVariables_DefaultFunc(t *testing.T) {
	content := models.Content{Title: "", Body: `{{ default "fallback" .Title }}`}
	got := applyTemplateVariables(content.Body, content)
	assert.Equal(t, "fallback", got)
}

func TestApplyTemplateVariables_DefaultFuncWithValue(t *testing.T) {
	content := models.Content{Title: "present", Body: `{{ default "fallback" .Title }}`}
	got := applyTemplateVariables(content.Body, content)
	assert.Equal(t, "present", got)
}

func TestApplyTemplateVariables_ContainsFunc(t *testing.T) {
	content := models.Content{Title: "hello world", Body: `{{ contains .Title "world" }}`}
	got := applyTemplateVariables(content.Body, content)
	assert.Equal(t, "true", got)
}

func TestApplyTemplateVariables_ReplaceFunc(t *testing.T) {
	content := models.Content{Title: "hello world", Body: `{{ replace .Title "world" "Go" }}`}
	got := applyTemplateVariables(content.Body, content)
	assert.Equal(t, "hello Go", got)
}

func TestSafeFuncs_HasAllKeys(t *testing.T) {
	fns := SafeFuncs()
	expected := []string{"upper", "lower", "trim", "short", "now", "date", "join", "replace", "contains", "default"}
	for _, key := range expected {
		_, ok := fns[key]
		assert.True(t, ok, "SafeFuncs missing key: %s", key)
	}
}

func TestSafeFuncs_NowAndDate(t *testing.T) {
	content := models.Content{Body: `{{ now | date }}`}
	got := applyTemplateVariables(content.Body, content)
	// Should return a date string in YYYY-MM-DD format
	assert.Regexp(t, `^\d{4}-\d{2}-\d{2}$`, got)
}
