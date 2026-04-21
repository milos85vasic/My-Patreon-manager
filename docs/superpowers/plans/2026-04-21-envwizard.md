# EnvWizard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build EnvWizard - interactive CLI/TUI/REST/Web/Mobile wizard that guides users through configuring all environment variables for My-Patreon-Manager, generates `.env` file.

**Architecture:** Shared Go library (`internal/envwizard/`) with 5 platform frontends (CLI, TUI, Web, REST API). Mobile is Flutter app communicating via REST.

**Tech Stack:** Go 1.26.1, bubbles (TUI), chi (REST), Flutter (mobile), testify (tests)

---

## File Structure

### New Files

```
cmd/envwizard/
├── main.go                 # Entry point with platform router
├── cli/
│   └── cli.go             # Terminal menu interface
├── tui/
│   └── tui.go             # Interactive TUI (bubbles)
├── web/
│   ├── server.go          # HTTP server
│   └── static/            # Embedded HTML/JS/CSS
└── api/
    └── api.go             # REST endpoints

internal/envwizard/
├── core/
│   ├── config.go          # EnvVar types
│   ├── wizard.go         # State machine
│   ├── profiles.go       # Profile management
│   └── generator.go      # .env file generation
├── definitions/
│   ├── vars.go            # All 50+ env var definitions
│   ├── categories.go      # Category definitions
│   └── validation.go      # Validation functions
└── ui/
    └── components.go      # Shared UI helpers

tests/
├── envwizard/
│   ├── core_test.go       # Core business logic
│   ├── definitions_test.go # Env var definitions
│   └── generator_test.go  # .env generation

cmd/envwizard/
├── cli_test.go           # CLI interface tests
├── api_test.go           # REST API tests
└── integration_test.go   # Full flow tests
```

---

## Implementation Tasks

### Task 0: .env Parser and Loader

**Files:**
- Create: `internal/envwizard/core/parser.go`
- Create: `internal/envwizard/core/parser_test.go`

- [ ] **Step 1: Write failing test for .env parsing**

```go
// internal/envwizard/core/parser_test.go
package core_test

import (
    "testing"
    "github.com/milosvasic/My-Patreon-Manager/internal/envwizard/core"
    "github.com/stretchr/testify/assert"
)

func TestParser_LoadEnvFile(t *testing.T) {
    content := `PORT=8080
# This is a comment
ADMIN_KEY=secret123
OPTIONAL_VAR=
`
    vars, err := core.ParseEnvFile(content)
    assert.NoError(t, err)
    assert.Equal(t, "8080", vars["PORT"])
    assert.Equal(t, "secret123", vars["ADMIN_KEY"])
    assert.Equal(t, "", vars["OPTIONAL_VAR"])
}

func TestParser_LoadFromPath(t *testing.T) {
    vars, err := core.LoadEnvFile(".env")
    assert.NoError(t, err)
    // If .env exists, values should be loaded
}

func TestParser_GenerateEnvFile(t *testing.T) {
    vars := map[string]string{
        "PORT": "3000",
        "# Comment": "",
    }
    content, err := core.GenerateEnvFile(vars)
    assert.NoError(t, err)
    assert.Contains(t, content, "PORT=3000")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /run/media/milosvasic/DATA4TB/Projects/My-Patreon-Manager && go test ./internal/envwizard/core/... -v -run TestParser
# Expected: FAIL - package doesn't exist yet
```

- [ ] **Step 3: Implement .env parser**

```go
// internal/envwizard/core/parser.go
package core

import (
    "bufio"
    "strings"
)

func ParseEnvFile(content string) (map[string]string, error) {
    vars := make(map[string]string)
    lines := strings.Split(content, "\n")
    for i, line := range lines {
        line = strings.TrimSpace(line)
        if line == "" || strings.HasPrefix(line, "#") {
            continue
        }
        if idx := strings.Index(line, "="); idx > 0 {
            key := strings.TrimSpace(line[:idx])
            val := strings.TrimSpace(line[idx+1:])
            vars[key] = val
        }
    }
    return vars, nil
}

func LoadEnvFile(path string) (map[string]string, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    return ParseEnvFile(string(data)), nil
}

func GenerateEnvFile(vars map[string]string) (string, error) {
    var builder strings.Builder
    for key, val := range vars {
        fmt.Fprintf(&builder, "%s=%s\n", key, val)
    }
    return builder.String(), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/envwizard/core/... -v -run TestParser
# Expected: PASS
```

- [ ] **Step 5: Commit**

```bash
git add internal/envwizard/core/parser.go
git commit -m "feat(envwizard): add .env file parser and loader"
```

---

### Task 1: Core Types and Definitions

**Files:**
- Create: `internal/envwizard/core/config.go`
- Create: `internal/envwizard/definitions/categories.go`
- Test: `internal/envwizard/definitions/definitions_test.go`

> **CRITICAL FEATURE:** Must support loading existing .env files at any time for continuing edits.

- [ ] **Step 1: Write failing test for EnvVar and Category types**

```go
// internal/envwizard/definitions/definitions_test.go
package definitions_test

import (
    "testing"
    "github.com/milosvasic/My-Patreon-Manager/internal/envwizard/core"
    "github.com/milosvasic/My-Patreon-Manager/internal/envwizard/definitions"
    "github.com/stretchr/testify/assert"
)

func TestEnvVarDefinitions_Count(t *testing.T) {
    vars := definitions.GetAll()
    assert.Greater(t, len(vars), 50, "should have 50+ env vars")
}

func TestCategories_AllCovered(t *testing.T) {
    cats := definitions.GetCategories()
    for _, v := range definitions.GetAll() {
        assert.NotNil(t, v.Category, "var %s should have category", v.Name)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /run/media/milosvasic/DATA4TB/Projects/My-Patreon-Manager && go test ./internal/envwizard/... -v
# Expected: FAIL - package doesn't exist yet
```

- [ ] **Step 3: Implement core types**

```go
// internal/envwizard/core/config.go
package core

type ValidationType string

const (
    ValidationRequired  ValidationType = "required"
    ValidationPort      ValidationType = "port"
    ValidationURL        ValidationType = "url"
    ValidationToken     ValidationType = "token"
    ValidationBoolean   ValidationType = "boolean"
    ValidationNumber    ValidationType = "number"
    ValidationRange     ValidationType = "range"
    ValidationCron      ValidationType = "cron"
    ValidationCustom    ValidationType = "custom"
)

type EnvVar struct {
    Name            string
    Description     string
    Category        *Category
    Required        bool
    Default        string
    Validation     ValidationType
    ValidationRule string
    URL             string
    CanGenerate     bool
    Secret          bool
    Example         string
}

type Category struct {
    ID          string
    Name        string
    Description string
    Order       int
}
```

```go
// internal/envwizard/definitions/categories.go
package definitions

func GetCategories() []*core.Category {
    return []*core.Category{
        {ID: "server", Name: "Server", Description: "Server configuration", Order: 1},
        {ID: "patreon", Name: "Patreon", Description: "Patreon API credentials", Order: 2},
        {ID: "database", Name: "Database", Description: "Database connection", Order: 3},
        {ID: "git", Name: "Git Providers", Description: "Git hosting service tokens", Order: 4},
        {ID: "content", Name: "Content Generation", Description: "LLM and content settings", Order: 5},
        {ID: "llmverifier", Name: "LLMsVerifier", Description: "LLM verification service", Order: 6},
        {ID: "illustration", Name: "Illustration", Description: "Image generation", Order: 7},
        {ID: "security", Name: "Security", Description: "API keys and secrets", Order: 8},
        {ID: "ratelimit", Name: "Rate Limiting", Description: "Rate limit configuration", Order: 9},
        {ID: "process", Name: "Process Pipeline", Description: "Process settings", Order: 10},
        {ID: "existing", Name: "Already Defined", Description: "Previously configured variables", Order: 99},
    }
}
```

```go
// internal/envwizard/definitions/vars.go
package definitions

func GetAll() []*core.EnvVar {
    cats := make(map[string]*core.Category)
    for _, c := range GetCategories() {
        cats[c.ID] = c
    }
    
    return []*core.EnvVar{
        // Server (Category 1)
        {Name: "PORT", Description: "HTTP server port", Category: cats["server"], Required: true, Default: "8080", Validation: core.ValidationPort, Example: "8080"},
        {Name: "GIN_MODE", Description: "Gin framework mode", Category: cats["server"], Required: false, Default: "debug", Validation: core.ValidationCustom, ValidationRule: "^(debug|release|test)$"},
        
        // Patreon (Category 2)
        {Name: "PATREON_CLIENT_ID", Description: "Your Patreon API client identifier", Category: cats["patreon"], Required: true, URL: "https://www.patreon.com/platform/documentation/clients"},
        {Name: "PATREON_CLIENT_SECRET", Description: "Your Patreon API client secret", Category: cats["patreon"], Required: true, Secret: true, URL: "https://www.patreon.com/platform/documentation/clients"},
        // ... add all 50+ vars following .env.example
        
        // Security - secrets that can be generated
        {Name: "HMAC_SECRET", Description: "Secret for HMAC-signed URLs", Category: cats["security"], Required: true, Secret: true, CanGenerate: true, Example: "openssl rand -hex 32"},
        {Name: "ADMIN_KEY", Description: "Admin API key", Category: cats["security"], Required: true, Secret: true, CanGenerate: true},
        {Name: "REVIEWER_KEY", Description: "Optional reviewer key for preview UI", Category: cats["security"], Required: false, Secret: true, CanGenerate: true},
    }
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /run/media/milosvasic/DATA4TB/Projects/My-Patreon-Manager && go test ./internal/envwizard/... -v
# Expected: PASS
```

- [ ] **Step 5: Commit**

```bash
git add internal/envwizard/
git commit -m "feat(envwizard): add core types and env var definitions"
```

---

### Task 2: State Machine and Wizard Logic

**Files:**
- Modify: `internal/envwizard/core/config.go`
- Create: `internal/envwizard/core/wizard.go`
- Test: `internal/envwizard/core/wizard_test.go`

- [ ] **Step 1: Write failing test for WizardState**

```go
// internal/envwizard/core/wizard_test.go
package core_test

import (
    "testing"
    "github.com/milosvasic/My-Patreon-Manager/internal/envwizard/core"
    "github.com/stretchr/testify/assert"
)

func TestWizardState_Navigation(t *testing.T) {
    w := core.NewWizard()
    assert.Equal(t, 0, w.CurrentStep())
    
    w.Next()
    assert.Equal(t, 1, w.CurrentStep())
    
    w.Previous()
    assert.Equal(t, 0, w.CurrentStep())
}

func TestWizardState_SetValue(t *testing.T) {
    w := core.NewWizard()
    w.SetValue("PORT", "3000")
    assert.Equal(t, "3000", w.GetValue("PORT"))
}

func TestWizardState_Skip(t *testing.T) {
    w := core.NewWizard()
    w.Skip("PORT")
    assert.True(t, w.IsSkipped("PORT"))
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/envwizard/core/... -v
# Expected: FAIL - Wizard type doesn't exist
```

- [ ] **Step 3: Implement WizardState**

```go
// internal/envwizard/core/wizard.go
package core

type WizardState struct {
    ProfileName     string
    CurrentStep    int
    History        []int
    Values         map[string]string
    Skipped        map[string]bool
    ValidationErr  map[string]error
    Modified       map[string]bool
}

func NewWizard() *WizardState {
    return &WizardState{
        Values:    make(map[string]string),
        Skipped:   make(map[string]bool),
        ValidationErr: make(map[string]error),
        Modified:  make(map[string]bool),
        History:   []int{0},
    }
}

func (w *WizardState) Next() {
    w.CurrentStep++
    w.History = append(w.History, w.CurrentStep)
}

func (w *WizardState) Previous() {
    if len(w.History) > 1 {
        w.History = w.History[:len(w.History)-1]
        w.CurrentStep = w.History[len(w.History)-1]
    }
}

func (w *WizardState) CurrentStep() int { return w.CurrentStep }
func (w *WizardState) SetValue(key, value string) { w.Values[key] = value; w.Modified[key] = true }
func (w *WizardState) GetValue(key string) string { return w.Values[key] }
func (w *WizardState) Skip(key string) { w.Skipped[key] = true }
func (w *WizardState) IsSkipped(key string) bool { return w.Skipped[key] }
func (w *WizardState) SetError(key string, err error) { w.ValidationErr[key] = err }
func (w *WizardState) GetError(key string) error { return w.ValidationErr[key] }
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/envwizard/core/... -v
# Expected: PASS
```

- [ ] **Step 5: Commit**

```bash
git add internal/envwizard/core/
git commit -m "feat(envwizard): add wizard state machine"
```

---

### Task 3: Profile Management

**Files:**
- Create: `internal/envwizard/core/profiles.go`
- Create: `internal/envwizard/core/profiles_test.go`

- [ ] **Step 1: Write failing test for Profile management**

```go
func TestProfile_SaveLoad(t *testing.T) {
    p := &core.Profile{
        Name: "development",
        Values: map[string]string{"PORT": "8080"},
    }
    err := core.SaveProfile(p)
    assert.NoError(t, err)
    
    loaded, err := core.LoadProfile("development")
    assert.NoError(t, err)
    assert.Equal(t, "8080", loaded.Values["PORT"])
    
    core.DeleteProfile("development")
}
```

- [ ] **Step 2: Run test - verify fails**

- [ ] **Step 3: Implement Profile storage**

```go
// Uses XDG_CONFIG_HOME or ~/.config/patreon-manager/profiles/
func SaveProfile(p *Profile) error { /* ... */ }
func LoadProfile(name string) (*Profile, error) { /* ... */ }
func ListProfiles() ([]string, error) { /* ... */ }
func DeleteProfile(name string) error { /* ... */ }
```

- [ ] **Step 4: Run test - verify passes**

- [ ] **Step 5: Commit**

---

### Task 4: .env File Generator

**Files:**
- Create: `internal/envwizard/core/generator.go`
- Create: `internal/envwizard/core/generator_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestGenerator_ProduceEnvFile(t *testing.T) {
    w := core.NewWizard()
    w.SetValue("PORT", "3000")
    w.SetValue("ADMIN_KEY", "secret123")
    
    gen := core.NewGenerator(w)
    output, err := gen.ProduceEnvFile()
    assert.NoError(t, err)
    assert.Contains(t, output, "PORT=3000")
    assert.NotContains(t, output, "secret123") // masked or excluded
}
```

- [ ] **Step 2: Run test - verify fails**

- [ ] **Step 3: Implement generator**

```go
// Handles:
// - Variable ordering (category order)
// - Comments (descriptions)
// - Secret masking option
// - Default value handling
// - Already-defined skip logic
```

- [ ] **Step 4: Run test - verify passes**

- [ ] **Step 5: Commit**

---

### Task 5: Validation Functions

**Files:**
- Create: `internal/envwizard/definitions/validation.go`
- Create: `internal/envwizard/definitions/validation_test.go`

- [ ] **Step 1: Write failing tests for all validation types**

```go
func TestValidatePort(t *testing.T) {
    assert.NoError(t, definitions.Validate("8080", definitions.ValidationPort, ""))
    assert.Error(t, definitions.Validate("0", definitions.ValidationPort, ""))
    assert.Error(t, definitions.Validate("70000", definitions.ValidationPort, ""))
}

func TestValidateBoolean(t *testing.T) {
    assert.NoError(t, definitions.Validate("true", definitions.ValidationBoolean, ""))
    assert.NoError(t, definitions.Validate("1", definitions.ValidationBoolean, ""))
    assert.NoError(t, definitions.Validate("false", definitions.ValidationBoolean, ""))
    assert.Error(t, definitions.Validate("maybe", definitions.ValidationBoolean, ""))
}
```

- [ ] **Step 2: Run tests - verify fails**

- [ ] **Step 3: Implement validation functions**

```go
func Validate(value string, validationType ValidationType, rule string) error {
    switch validationType {
    case ValidationPort: return validatePort(value)
    case ValidationBoolean: return validateBoolean(value)
    case ValidationRequired: return validateRequired(value)
    // ... all types
    }
}
```

- [ ] **Step 4: Run tests - verify passes**

- [ ] **Step 5: Commit**

---

### Task 6: CLI Interface

**Files:**
- Create: `cmd/envwizard/main.go`
- Create: `cmd/envwizard/cli/cli.go`
- Create: `cmd/envwizard/cli/cli_test.go`

- [ ] **Step 1: Write failing test for CLI flow**

```go
func TestCLI_SetValue(t *testing.T) {
    // Simulate: PORT=8080 input
    input := "8080\n"
    output, err := runCLIWithInput(input, "set", "PORT")
    assert.NoError(t, err)
    assert.Contains(t, output, "PORT set to 8080")
}
```

- [ ] **Step 2: Run test - verify fails**

- [ ] **Step 3: Implement CLI**

```go
// Simple numbered menu interface
// - List categories
// - Set variable value  
// - Show summary
// - Save .env
// - Next/Previous navigation
```

- [ ] **Step 4: Run test - verify passes**

- [ ] **Step 5: Commit**

---

### Task 7: TUI Interface

**Files:**
- Create: `cmd/envwizard/tui/tui.go`
- Create: `cmd/envwizard/tui/tui_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestTUI_RenderCategory(t *testing.T) {
    ui := tui.New()
    screen := ui.RenderCategory("server")
    assert.Contains(t, screen, "PORT")
}
```

- [ ] **Step 2: Run test - verify fails**

- [ ] **Step 3: Implement TUI using bubbles**

```go
// Form-based UI with:
// - Category list view
// - Variable input form
// - Progress indicator
// - Validation feedback
// - Help/description panel
```

- [ ] **Step 4: Run test - verify passes**

- [ ] **Step 5: Commit**

---

### Task 8: REST API Server

**Files:**
- Create: `cmd/envwizard/api/api.go`
- Create: `cmd/envwizard/api/api_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestAPI_SetValue(t *testing.T) {
    srv := api.NewServer()
    resp := httptest.NewRequest("POST", "/api/vars/PORT", strings.NewReader(`{"value":"3000"}`))
    w := httptest.NewRecorder()
    srv.ServeHTTP(w, resp)
    assert.Equal(t, 200, w.Code)
}
```

- [ ] **Step 2: Run test - verify fails**

- [ ] **Step 3: Implement REST API**

```go
// Endpoints:
// GET  /api/categories           - List categories
// GET  /api/vars                - List all variables
// GET  /api/vars/:name         - Get single variable
// POST /api/vars/:name         - Set value
// POST /api/skip/:name         - Skip variable
// POST /api/save              - Save .env file
// GET  /api/profiles           - List profiles
// POST /api/profiles          - Save profile
// GET  /api/wizard/state      - Get current state
// POST /api/wizard/next      - Next step
// POST /api/wizard/prev      - Previous step
```

- [ ] **Step 4: Run test - verify passes**

- [ ] **Step 5: Commit**

---

### Task 9: Web Server with Embedded UI

**Files:**
- Create: `cmd/envwizard/web/server.go`
- Create: `cmd/envwizard/web/static/index.html` (embedded)
- Create: `cmd/envwizard/web/web_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestWeb_IndexPage(t *testing.T) {
    srv := web.NewServer()
    resp := httptest.NewRequest("GET", "/", nil)
    w := httptest.NewRecorder()
    srv.ServeHTTP(w, resp)
    assert.Equal(t, 200, w.Code)
    assert.Contains(t, w.Body.String(), "EnvWizard")
}
```

- [ ] **Step 2: Run test - verify fails**

- [ ] **Step 3: Implement web server + embedded SPA**

```go
// Serves REST API at /api/*
// Serves SPA at /*
// Uses go:embed for static files
```

- [ ] **Step 4: Run test - verify passes**

- [ ] **Step 5: Commit**

---

### Task 10: Main Entry Point with Platform Router

**Files:**
- Modify: `cmd/envwizard/main.go`

- [ ] **Step 1: Write failing test**

```bash
./envwizard --help
# Should show: cli, tui, web, api modes
```

- [ ] **Step 2: Run test - verify fails**

- [ ] **Step 3: Implement main with flags**

```bash
./envwizard              # Auto-detect (CLI or TUI)
./envwizard --cli       # Force CLI mode
./envwizard --tui       # Force TUI mode  
./envwizard --web       # Web UI on :8080
./envwizard --api       # REST API on :8081
./envwizard --profile dev  # Use profile
./envwizard --output .env   # Custom output path
```

- [ ] **Step 4: Run test - verify passes**

- [ ] **Step 5: Commit**

---

### Task 11: Flutter Mobile App (Separate Repo)

**Files:**
- Create: Flutter project at `Upstreams/flutter-envwizard/` (or separate mirror)

Note: This task creates the Flutter app that connects to REST API. The Flutter app itself is out of scope for this repo - it will be a separate repository following the LLMProvider/Models mirror pattern.

- [ ] **Step 1: Document Flutter integration**

Create `docs/superpowers/specs/2026-04-21-envwizard-flutter.md` with:
- Flutter project structure
- REST API integration
- Error tracking (sentry_flutter)
- Build instructions

- [ ] **Step 2: Commit**

```bash
git add docs/superpowers/specs/2026-04-21-envwizard-flutter.md
git commit -m "docs(envwizard): add Flutter mobile app spec"
```

---

### Task 12: Integration Tests

**Files:**
- Create: `tests/envwizard/integration_test.go`

- [ ] **Step 1: Write full flow integration test**

```go
func TestFullWizardFlow(t *testing.T) {
    // 1. Create wizard
    w := core.NewWizard()
    
    // 2. Navigate through all categories
    for w.CurrentStep() < totalSteps {
        // 3. Set required values
        w.SetValue("PORT", "8080")
        // 4. Skip optional
        w.Skip("REVIEWER_KEY")
        w.Next()
    }
    
    // 5. Generate .env
    gen := core.NewGenerator(w)
    output, _ := gen.ProduceEnvFile()
    
    // 6. Validate output
    assert.Contains(t, output, "PORT=8080")
    assert.NotContains(t, output, "REVIEWER_KEY")
}
```

- [ ] **Step 2: Run test - verify passes**

- [ ] **Step 3: Commit**

---

### Task 13: Documentation and User Guides

**Files:**
- Create: `docs/manuals/envwizard.md`

- [ ] **Step 1: Write comprehensive user guide**

```markdown
# EnvWizard User Guide

## Installation
## First Run
## Configuration Options
## Profiles
## Output Formats
## Troubleshooting
```

- [ ] **Step 2: Commit**

---

## Execution Choice

**Plan complete and saved to `docs/superpowers/plans/2026-04-21-envwizard.md`. Two execution options:**

**1. Subagent-Driven (recommended)** - Dispatch fresh subagent per task, review between tasks

**2. Inline Execution** - Execute tasks in this session using executing_plans, batch execution with checkpoints

Which approach?
