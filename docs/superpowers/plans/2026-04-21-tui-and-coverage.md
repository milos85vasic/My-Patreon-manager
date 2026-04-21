# TUI Frontend + Coverage Gap Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a tview-based TUI frontend to EnvWizard and fix coverage gaps in `cmd/cli` (95.7%) and `cmd/server` (96.5%) to reach 100%.

**Architecture:** TUI lives in `cmd/envwizard/tui/` using `tview` for rich terminal widgets. It consumes the same `core.Wizard` state machine as CLI/Web. Coverage fixes add targeted tests for uncovered branches in existing `*_test.go` files.

**Tech Stack:** Go 1.26.1, tview, tcell, testify, existing DI patterns

---

## File Structure

### New Files

```
cmd/envwizard/tui/
├── tui.go          # TUI application with all screens
└── tui_test.go     # Tests for TUI screens and navigation
```

### Modified Files

```
cmd/envwizard/main.go              # Add --tui flag and auto-detect
cmd/cli/llmsverifier_boot_test.go  # Coverage for findBootstrapScript
cmd/cli/merge_history_test.go      # Coverage for cleanup, processing, empty revisions
cmd/cli/migrate_test.go            # Coverage for parseMigrateDownFlags, printMigrationStatus
cmd/server/coverage_gaps_test.go   # Coverage for runServer, setupRouter edge cases
go.mod                             # Add rivo/tview dependency
go.sum                             # Updated checksums
```

---

## Task 0: Add tview Dependency

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add tview dependency**

Run:
```bash
go get github.com/rivo/tview@latest
go mod tidy
```

- [ ] **Step 2: Verify dependency resolves**

Run:
```bash
go build ./cmd/envwizard/...
```
Expected: success (no import errors)

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add tview dependency for TUI frontend"
```

---

## Task 1: TUI Application — Core Structure and Welcome Screen

**Files:**
- Create: `cmd/envwizard/tui/tui.go`
- Create: `cmd/envwizard/tui/tui_test.go`

- [ ] **Step 1: Write failing test for TUI creation**

Create `cmd/envwizard/tui/tui_test.go`:

```go
package tui_test

import (
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/cmd/envwizard/tui"
	"github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var tuiTestVars = []*core.EnvVar{
	{Name: "PORT", Description: "HTTP port", Required: true, Validation: core.ValidationPort, Default: "8080"},
	{Name: "ADMIN_KEY", Description: "Admin key", Required: true, Secret: true},
	{Name: "GITHUB_TOKEN", Description: "GitHub token", Required: false, Secret: true},
	{Name: "DEBUG", Description: "Debug mode", Required: false, Default: "false", Validation: core.ValidationBoolean},
}

func TestNewTUI(t *testing.T) {
	w := core.NewWizard(tuiTestVars)
	app := tui.New(w)
	require.NotNil(t, app)
	assert.Equal(t, tui.ScreenWelcome, app.CurrentScreen())
}

func TestNewTUI_NilWizardPanics(t *testing.T) {
	assert.Panics(t, func() { tui.New(nil) })
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/envwizard/tui/... -run "TestNewTUI" -v`
Expected: FAIL — package doesn't exist

- [ ] **Step 3: Write TUI core structure and welcome screen**

Create `cmd/envwizard/tui/tui.go`:

```go
package tui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/core"
	"github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/definitions"
)

type Screen int

const (
	ScreenWelcome  Screen = iota
	ScreenCategories
	ScreenVariable
	ScreenSummary
	ScreenSave
	ScreenProfileSave
)

type TUI struct {
	app      *tview.Application
	wizard   *core.Wizard
	pages    *tview.Pages
	screen   Screen
	envPath  string

	welcomeFlex    *tview.Flex
	categoryTable  *tview.Table
	variableForm   *tview.Form
	summaryTable   *tview.Table
	saveForm       *tview.Form
	profileForm    *tview.Form
	statusBar      *tview.TextView

	currentCategoryIdx int
	currentVarIdx      int
	categoryVars       []*core.EnvVar
	varInputField      *tview.InputField
	varValidationText  *tview.TextView
}

func New(w *core.Wizard) *TUI {
	if w == nil {
		panic("tui: wizard must not be nil")
	}
	t := &TUI{
		app:    tview.NewApplication(),
		wizard: w,
		pages:  tview.NewPages(),
		screen: ScreenWelcome,
	}

	t.app.SetInputCapture(t.globalKeyHandler)
	t.buildWelcomeScreen()
	t.buildCategoryScreen()
	t.buildVariableScreen()
	t.buildSummaryScreen()
	t.buildSaveScreen()
	t.buildProfileScreen()

	t.pages.AddPage("welcome", t.welcomeFlex, true, true)

	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(t.pages, 0, 1, true).
		AddItem(t.statusBar, 1, 0, false)

	t.app.SetRoot(root, true)
	t.updateStatusBar()
	return t
}

func (t *TUI) Run() error {
	return t.app.Run()
}

func (t *TUI) CurrentScreen() Screen {
	return t.screen
}

func (t *TUI) updateStatusBar() {
	completed, total := t.wizard.Progress()
	t.statusBar.SetText(
		fmt.Sprintf(" [green]EnvWizard[-] | Step %d/%d | [yellow]%d completed[-] | Ctrl+S=Save Ctrl+Q=Quit Ctrl+R=Summary",
			t.wizard.Step, total, completed))
}

func (t *TUI) switchScreen(s Screen) {
	t.screen = s
	switch s {
	case ScreenWelcome:
		t.pages.SwitchToPage("welcome")
	case ScreenCategories:
		t.buildCategoryTable()
		t.pages.SwitchToPage("categories")
	case ScreenVariable:
		t.buildVariableForm()
		t.pages.SwitchToPage("variable")
	case ScreenSummary:
		t.buildSummaryTable()
		t.pages.SwitchToPage("summary")
	case ScreenSave:
		t.pages.SwitchToPage("save")
	case ScreenProfileSave:
		t.pages.SwitchToPage("profile")
	}
	t.updateStatusBar()
	t.app.SetFocus(t.pages)
}

func (t *TUI) globalKeyHandler(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyCtrlQ:
		t.app.Stop()
		return nil
	case tcell.KeyCtrlS:
		t.switchScreen(ScreenSave)
		return nil
	case tcell.KeyCtrlR:
		t.switchScreen(ScreenSummary)
		return nil
	}
	return event
}

func (t *TUI) buildWelcomeScreen() {
	title := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)
	fmt.Fprintf(title, "\n[green::b]EnvWizard[-:-:-]\n\n[yellow]Interactive .env Configuration Wizard[-]\n\n[blue]My-Patreon-Manager[-]\n\n")

	profiles, _ := core.ListProfiles()
	options := []string{"New Configuration"}
	options = append(options, profiles...)

	profileDropdown := tview.NewDropDown().SetLabel("Profile: ").SetOptions(options...)
	envInput := tview.NewInputField().SetLabel(".env path: ").SetPlaceholder("(optional) path to existing .env file")

	startBtn := tview.NewButton("Start").SetSelectedFunc(func() {
		_, profileOpt := profileDropdown.GetCurrentOption()
		if profileOpt != "New Configuration" && profileOpt != "" {
			p, err := core.LoadProfile(profileOpt)
			if err == nil {
				for k, v := range p.Values {
					t.wizard.SetValue(k, v)
				}
			}
		}
		if t.envPath != "" {
			w2, err := core.NewWizardFromEnvFile(t.wizard.Vars, t.envPath)
			if err == nil {
				t.wizard.Values = w2.Values
				t.wizard.Modified = w2.Modified
			}
		}
		t.switchScreen(ScreenCategories)
	})

	envInput.SetChangedFunc(func(text string) {
		t.envPath = text
	})

	quitBtn := tview.NewButton("Quit").SetSelectedFunc(func() {
		t.app.Stop()
	})

	btnRow := tview.NewFlex().
		AddItem(startBtn, 10, 0, true).
		AddItem(nil, 2, 0, false).
		AddItem(quitBtn, 10, 0, false)

	form := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(title, 7, 0, false).
		AddItem(profileDropdown, 1, 0, true).
		AddItem(nil, 1, 0, false).
		AddItem(envInput, 1, 0, true).
		AddItem(nil, 1, 0, false).
		AddItem(btnRow, 1, 0, true)

	t.welcomeFlex = form
}

func (t *TUI) buildCategoryScreen() {
	t.categoryTable = tview.NewTable().
		SetBorders(true).
		SetSelectable(true, false).
		SetSelectedFunc(func(row, col int) {
			t.currentCategoryIdx = row - 1
			cats := definitions.GetCategories()
			if t.currentCategoryIdx < 0 || t.currentCategoryIdx >= len(cats) {
				return
			}
			cat := cats[t.currentCategoryIdx]
			t.categoryVars = make([]*core.EnvVar, 0)
			for _, v := range t.wizard.Vars {
				if v.Category != nil && v.Category.ID == cat.ID {
					t.categoryVars = append(t.categoryVars, v)
				}
			}
			if len(t.categoryVars) > 0 {
				t.currentVarIdx = 0
				t.switchScreen(ScreenVariable)
			}
		})

	wrapper := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewTextView().SetTextAlign(tview.AlignCenter).SetText("[green::b]Categories[-:-:-]"), 1, 0, false).
		AddItem(t.categoryTable, 0, 1, true)

	t.pages.AddPage("categories", wrapper, true, false)
}

func (t *TUI) buildCategoryTable() {
	t.categoryTable.Clear()
	cats := definitions.GetCategories()
	t.categoryTable.SetCell(0, 0, tview.NewTableCell("[::b]Category[-:-:-]").SetTextColor(tcell.ColorYellow))
	t.categoryTable.SetCell(0, 1, tview.NewTableCell("[::b]Description[-:-:-]").SetTextColor(tcell.ColorYellow))
	t.categoryTable.SetCell(0, 2, tview.NewTableCell("[::b]Progress[-:-:-]").SetTextColor(tcell.ColorYellow))

	for i, cat := range cats {
		row := i + 1
		total := 0
		set := 0
		for _, v := range t.wizard.Vars {
			if v.Category != nil && v.Category.ID == cat.ID {
				total++
				if t.wizard.IsSet(v.Name) || t.wizard.IsSkipped(v.Name) {
					set++
				}
			}
		}
		progress := fmt.Sprintf("%d/%d", set, total)
		t.categoryTable.SetCell(row, 0, tview.NewTableCell(cat.Name))
		t.categoryTable.SetCell(row, 1, tview.NewTableCell(cat.Description))
		color := "red"
		if set == total && total > 0 {
			color = "green"
		} else if set > 0 {
			color = "yellow"
		}
		t.categoryTable.SetCell(row, 2, tview.NewTableCell(fmt.Sprintf("[%s]%s[-]", color, progress)))
	}
}

func (t *TUI) buildVariableScreen() {
	t.variableForm = tview.NewForm()
	t.varValidationText = tview.NewTextView().SetDynamicColors(true)
	t.varInputField = tview.NewInputField()

	wrapper := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(t.variableForm, 0, 1, true).
		AddItem(t.varValidationText, 1, 0, false)

	t.pages.AddPage("variable", wrapper, true, false)
}

func (t *TUI) buildVariableForm() {
	t.variableForm.Clear(false)
	t.varValidationText.SetText("")

	if t.currentVarIdx < 0 || t.currentVarIdx >= len(t.categoryVars) {
		return
	}
	v := t.categoryVars[t.currentVarIdx]

	header := v.Name
	if v.Required {
		header += " [red](required)[-]"
	}
	if v.Secret {
		header += " [purple](secret)[-]"
	}
	badge := ""
	if v.Category != nil {
		badge = v.Category.Name
	}
	t.variableForm.SetTitle(fmt.Sprintf("[ %s ] %s — Step %d/%d", badge, header, t.currentVarIdx+1, len(t.categoryVars)))

	t.varInputField = tview.NewInputField().
		SetLabel("Value: ").
		SetPlaceholder(v.Description)
	if v.Secret {
		t.varInputField.SetMaskCharacter('*')
	}

	existingVal := t.wizard.GetValue(v.Name)
	if existingVal != "" {
		t.varInputField.SetText(existingVal)
	}

	t.varInputField.SetChangedFunc(func(text string) {
		if text == "" {
			t.varValidationText.SetText("")
			return
		}
		if err := definitions.ValidateValue(v, text); err != nil {
			t.varValidationText.SetText(fmt.Sprintf("[red]%s[-]", err.Error()))
		} else {
			t.varValidationText.SetText("[green]Valid[-]")
		}
	})

	t.variableForm.AddFormItem(t.varInputField)

	t.variableForm.AddButton("Next", func() {
		val := t.varInputField.GetText()
		if val != "" {
			t.wizard.SetValue(v.Name, val)
		}
		if t.currentVarIdx < len(t.categoryVars)-1 {
			t.currentVarIdx++
			t.buildVariableForm()
			t.app.SetFocus(t.variableForm)
		} else {
			t.switchScreen(ScreenCategories)
		}
		t.updateStatusBar()
	})

	t.variableForm.AddButton("Skip", func() {
		t.wizard.Skip(v.Name)
		if t.currentVarIdx < len(t.categoryVars)-1 {
			t.currentVarIdx++
			t.buildVariableForm()
			t.app.SetFocus(t.variableForm)
		} else {
			t.switchScreen(ScreenCategories)
		}
		t.updateStatusBar()
	})

	t.variableForm.AddButton("Previous", func() {
		if t.currentVarIdx > 0 {
			t.currentVarIdx--
			t.buildVariableForm()
			t.app.SetFocus(t.variableForm)
		} else {
			t.switchScreen(ScreenCategories)
		}
	})
}

func (t *TUI) buildSummaryScreen() {
	t.summaryTable = tview.NewTable().SetBorders(true)

	wrapper := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewTextView().SetTextAlign(tview.AlignCenter).SetText("[green::b]Summary[-:-:-]"), 1, 0, false).
		AddItem(t.summaryTable, 0, 1, true)

	t.pages.AddPage("summary", wrapper, true, false)
}

func (t *TUI) buildSummaryTable() {
	t.summaryTable.Clear()
	t.summaryTable.SetCell(0, 0, tview.NewTableCell("[::b]Variable[-:-:-]").SetTextColor(tcell.ColorYellow))
	t.summaryTable.SetCell(0, 1, tview.NewTableCell("[::b]Value[-:-:-]").SetTextColor(tcell.ColorYellow))
	t.summaryTable.SetCell(0, 2, tview.NewTableCell("[::b]Status[-:-:-]").SetTextColor(tcell.ColorYellow))

	row := 1
	for _, v := range t.wizard.Vars {
		val := t.wizard.GetValue(v.Name)
		status := "[gray]missing[-]"
		if t.wizard.IsSet(v.Name) {
			if v.Secret {
				val = "********"
			}
			status = "[green]set[-]"
		} else if t.wizard.IsSkipped(v.Name) {
			status = "[yellow]skipped[-]"
		} else if v.Required {
			status = "[red]MISSING (required)[-]"
		}
		t.summaryTable.SetCell(row, 0, tview.NewTableCell(v.Name))
		t.summaryTable.SetCell(row, 1, tview.NewTableCell(val))
		t.summaryTable.SetCell(row, 2, tview.NewTableCell(status))
		row++
	}
}

func (t *TUI) buildSaveScreen() {
	t.saveForm = tview.NewForm()
	pathInput := tview.NewInputField().SetLabel("Save to: ").SetText(".env")

	t.saveForm.AddFormItem(pathInput)
	t.saveForm.AddButton("Save", func() {
		gen := core.NewGenerator(t.wizard)
		if err := gen.SaveToPath(pathInput.GetText(), false); err != nil {
			t.varValidationText.SetText(fmt.Sprintf("[red]Error: %s[-]", err.Error()))
			return
		}
		t.switchScreen(ScreenProfileSave)
	})
	t.saveForm.AddButton("Back", func() {
		t.switchScreen(ScreenCategories)
	})

	t.pages.AddPage("save", t.saveForm, true, false)
}

func (t *TUI) buildProfileScreen() {
	t.profileForm = tview.NewForm()
	nameInput := tview.NewInputField().SetLabel("Profile name: ").SetPlaceholder("(optional) save as profile")

	t.profileForm.AddFormItem(nameInput)
	t.profileForm.AddButton("Save Profile", func() {
		name := nameInput.GetText()
		if name != "" {
			p := &core.Profile{Name: name, Values: t.wizard.Values}
			core.SaveProfile(p)
		}
		t.app.Stop()
	})
	t.profileForm.AddButton("Skip & Exit", func() {
		t.app.Stop()
	})
	t.profileForm.AddButton("Back", func() {
		t.switchScreen(ScreenCategories)
	})

	t.pages.AddPage("profile", t.profileForm, true, false)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./cmd/envwizard/tui/... -run "TestNewTUI" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/envwizard/tui/tui.go cmd/envwizard/tui/tui_test.go
git commit -m "feat(envwizard): add tview TUI with all wizard screens"
```

---

## Task 2: TUI Navigation Tests

**Files:**
- Modify: `cmd/envwizard/tui/tui_test.go`

- [ ] **Step 1: Write navigation and screen transition tests**

Append to `cmd/envwizard/tui/tui_test.go`:

```go
func TestTUI_SwitchToCategories(t *testing.T) {
	w := core.NewWizard(tuiTestVars)
	app := tui.New(w)
	app.SwitchScreenForTest(tui.ScreenCategories)
	assert.Equal(t, tui.ScreenCategories, app.CurrentScreen())
}

func TestTUI_SwitchToSummary(t *testing.T) {
	w := core.NewWizard(tuiTestVars)
	app := tui.New(w)
	app.SwitchScreenForTest(tui.ScreenSummary)
	assert.Equal(t, tui.ScreenSummary, app.CurrentScreen())
}

func TestTUI_SwitchToSave(t *testing.T) {
	w := core.NewWizard(tuiTestVars)
	app := tui.New(w)
	app.SwitchScreenForTest(tui.ScreenSave)
	assert.Equal(t, tui.ScreenSave, app.CurrentScreen())
}

func TestTUI_SwitchToProfile(t *testing.T) {
	w := core.NewWizard(tuiTestVars)
	app := tui.New(w)
	app.SwitchScreenForTest(tui.ScreenProfileSave)
	assert.Equal(t, tui.ScreenProfileSave, app.CurrentScreen())
}

func TestTUI_SetValueViaWizard(t *testing.T) {
	w := core.NewWizard(tuiTestVars)
	app := tui.New(w)
	app.SwitchScreenForTest(tui.ScreenCategories)
	assert.Equal(t, tui.ScreenCategories, app.CurrentScreen())
	w.SetValue("PORT", "9090")
	assert.Equal(t, "9090", w.GetValue("PORT"))
}

func TestTUI_WizardProgress(t *testing.T) {
	w := core.NewWizard(tuiTestVars)
	app := tui.New(w)
	completed, total := w.Progress()
	assert.Equal(t, 0, completed)
	assert.Equal(t, 4, total)
	w.SetValue("PORT", "8080")
	completed, total = w.Progress()
	assert.Equal(t, 1, completed)
}
```

- [ ] **Step 2: Add test helper method to TUI**

Add to `cmd/envwizard/tui/tui.go`:

```go
func (t *TUI) SwitchScreenForTest(s Screen) {
	t.switchScreen(s)
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./cmd/envwizard/tui/... -v`
Expected: all PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/envwizard/tui/tui.go cmd/envwizard/tui/tui_test.go
git commit -m "test(envwizard): add TUI navigation and screen transition tests"
```

---

## Task 3: Wire TUI into Main Entry Point

**Files:**
- Modify: `cmd/envwizard/main.go`

- [ ] **Step 1: Add --tui flag and auto-detection**

Read the current `cmd/envwizard/main.go` and modify it to:

1. Add import `"github.com/milos85vasic/My-Patreon-Manager/cmd/envwizard/tui"`
2. Add `tuiMode bool` flag: `flag.BoolVar(&tuiMode, "tui", false, "Start TUI mode")`
3. Add a case in the switch for `tuiMode`:

```go
case tuiMode:
	t := tui.New(wizard)
	if runErr := t.Run(); runErr != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", runErr)
		os.Exit(1)
	}
```

The full modified switch block should be:

```go
switch {
case webMode:
	srv := web.NewServer(wizard)
	fmt.Printf("EnvWizard web UI on %s\n", webAddr)
	if listenErr := http.ListenAndServe(webAddr, srv); listenErr != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", listenErr)
		os.Exit(1)
	}
case apiMode:
	srv := envapi.NewServer(wizard)
	fmt.Printf("EnvWizard REST API on %s\n", apiAddr)
	if listenErr := http.ListenAndServe(apiAddr, srv); listenErr != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", listenErr)
		os.Exit(1)
	}
case tuiMode:
	t := tui.New(wizard)
	if runErr := t.Run(); runErr != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", runErr)
		os.Exit(1)
	}
default:
	c := cli.New(wizard)
	if runErr := c.Run(); runErr != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", runErr)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Build and verify**

Run: `go build ./cmd/envwizard/...`
Expected: success

- [ ] **Step 3: Commit**

```bash
git add cmd/envwizard/main.go
git commit -m "feat(envwizard): wire TUI mode via --tui flag"
```

---

## Task 4: Coverage — cmd/cli findBootstrapScript

**Files:**
- Modify: `cmd/cli/llmsverifier_boot_test.go`

- [ ] **Step 1: Add findBootstrapScript coverage tests**

Append to `cmd/cli/llmsverifier_boot_test.go`:

```go
func TestFindBootstrapScript_FoundInCurrentDir(t *testing.T) {
	dir := t.TempDir()
	scriptDir := filepath.Join(dir, "scripts")
	require.NoError(t, os.Mkdir(scriptDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(scriptDir, "llmsverifier.sh"), []byte("#!/bin/bash"), 0644))

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer os.Chdir(origWd)

	result := findBootstrapScript()
	assert.Contains(t, result, "llmsverifier.sh")
}

func TestFindBootstrapScript_FoundInParentDir(t *testing.T) {
	dir := t.TempDir()
	scriptDir := filepath.Join(dir, "scripts")
	require.NoError(t, os.Mkdir(scriptDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(scriptDir, "llmsverifier.sh"), []byte("#!/bin/bash"), 0644))

	subDir := filepath.Join(dir, "subdir")
	require.NoError(t, os.Mkdir(subDir, 0755))

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(subDir))
	defer os.Chdir(origWd)

	result := findBootstrapScript()
	assert.Contains(t, result, "llmsverifier.sh")
}

func TestFindBootstrapScript_NotFound(t *testing.T) {
	dir := t.TempDir()

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer os.Chdir(origWd)

	result := findBootstrapScript()
	assert.Empty(t, result)
}
```

Add imports at top of test file if not already present: `"os"`, `"path/filepath"`, `"github.com/stretchr/testify/require"`.

- [ ] **Step 2: Run tests**

Run: `go test ./cmd/cli/... -run "TestFindBootstrapScript" -v`
Expected: PASS

- [ ] **Step 3: Check coverage**

Run: `go test -race -coverprofile=/tmp/cli_cov.out ./cmd/cli/ && go tool cover -func=/tmp/cli_cov.out | grep findBootstrapScript`
Expected: 100%

- [ ] **Step 4: Commit**

```bash
git add cmd/cli/llmsverifier_boot_test.go
git commit -m "test(cli): add findBootstrapScript coverage tests"
```

---

## Task 5: Coverage — cmd/cli merge_history

**Files:**
- Modify: `cmd/cli/merge_history_test.go`

- [ ] **Step 1: Add merge_history coverage tests**

The existing tests already cover the happy path. Append these edge-case tests to `cmd/cli/merge_history_test.go`:

```go
func TestRunMergeHistory_OldRepoProcessing(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	seedMergeRepo(t, db, "old", "processing-repo")
	seedMergeRepo(t, db, "new", "new-repo")

	_, err := db.DB().ExecContext(ctx,
		`UPDATE repositories SET process_state = 'processing' WHERE id = ?`, "old")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runMergeHistory(ctx, db, []string{"old", "new"}, &buf, discardMergeLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "currently processing")
}

func TestRunMergeHistory_NewRepoHasRevisions(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	seedMergeRepo(t, db, "old", "old-repo")
	seedMergeRepo(t, db, "new", "new-repo")
	seedMergeRev(t, db, "rx", "new", 1)

	var buf bytes.Buffer
	err := runMergeHistory(ctx, db, []string{"old", "new"}, &buf, discardMergeLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already has")
}

func TestRunMergeHistory_EmptyOldRevisions(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	seedMergeRepo(t, db, "old", "old-repo")
	seedMergeRepo(t, db, "new", "new-repo")

	var buf bytes.Buffer
	err := runMergeHistory(ctx, db, []string{"old", "new"}, &buf, discardMergeLogger())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "merged 0 revisions")
}

func TestRunMergeHistory_CleanupWithFiles(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	seedMergeRepo(t, db, "old", "old-repo")
	seedMergeRepo(t, db, "new", "new-repo")
	seedMergeRev(t, db, "r1", "old", 1)

	tmpFile := filepath.Join(t.TempDir(), "illustration.png")
	require.NoError(t, os.WriteFile(tmpFile, []byte("img"), 0644))

	_, err := db.DB().ExecContext(ctx,
		`INSERT INTO illustrations (id, repository_id, file_path, prompt, status) VALUES (?,?,?,?,?)`,
		"ill1", "old", tmpFile, "test", "completed")
	require.NoError(t, err)

	var unlinked int
	origUnlink := unlinkIllustrationFile
	unlinkIllustrationFile = func(p string) error {
		unlinked++
		return origUnlink(p)
	}
	defer func() { unlinkIllustrationFile = origUnlink }()

	var buf bytes.Buffer
	err = runMergeHistory(ctx, db, []string{"old", "new", "--cleanup"}, &buf, discardMergeLogger())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "merged 1 revisions")
	assert.Contains(t, buf.String(), "unlinked 1 illustration")
	assert.Equal(t, 1, unlinked)
}

func TestRunMergeHistory_CleanupWithUnlinkErrors(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	seedMergeRepo(t, db, "old", "old-repo")
	seedMergeRepo(t, db, "new", "new-repo")
	seedMergeRev(t, db, "r1", "old", 1)

	_, err := db.DB().ExecContext(ctx,
		`INSERT INTO illustrations (id, repository_id, file_path, prompt, status) VALUES (?,?,?,?,?)`,
		"ill1", "old", "/fake/path.png", "test", "completed")
	require.NoError(t, err)

	origUnlink := unlinkIllustrationFile
	unlinkIllustrationFile = func(p string) error {
		return fmt.Errorf("permission denied")
	}
	defer func() { unlinkIllustrationFile = origUnlink }()

	var buf bytes.Buffer
	err = runMergeHistory(ctx, db, []string{"old", "new", "--cleanup"}, &buf, discardMergeLogger())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "1 cleanup error")
}

func TestRunMergeHistory_NilLogger(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	seedMergeRepo(t, db, "old", "old-repo")
	seedMergeRepo(t, db, "new", "new-repo")
	seedMergeRev(t, db, "r1", "old", 1)

	var buf bytes.Buffer
	err := runMergeHistory(ctx, db, []string{"old", "new"}, &buf, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "merged 1 revisions")
}

func TestRunMergeHistory_SameRepoIDs(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	var buf bytes.Buffer
	err := runMergeHistory(context.Background(), db, []string{"same", "same"}, &buf, discardMergeLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must differ")
}
```

Ensure `"fmt"`, `"os"`, `"path/filepath"` are in the import block.

- [ ] **Step 2: Run tests**

Run: `go test ./cmd/cli/... -run "TestRunMergeHistory_(OldRepoProcessing|NewRepoHasRevisions|EmptyOldRevisions|CleanupWithFiles|CleanupWithUnlinkErrors|NilLogger|SameRepoIDs)" -v`
Expected: all PASS

- [ ] **Step 3: Check coverage**

Run: `go test -race -coverprofile=/tmp/cli_cov.out ./cmd/cli/ && go tool cover -func=/tmp/cli_cov.out | grep runMergeHistory`
Expected: significantly improved, approaching 95%+

- [ ] **Step 4: Commit**

```bash
git add cmd/cli/merge_history_test.go
git commit -m "test(cli): add merge_history coverage for processing, cleanup, edge cases"
```

---

## Task 6: Coverage — cmd/cli migrate.go

**Files:**
- Modify: `cmd/cli/migrate_test.go`

- [ ] **Step 1: Add parseMigrateDownFlags and printMigrationStatus coverage tests**

Append to `cmd/cli/migrate_test.go`:

```go
func TestParseMigrateDownFlags_BackupToWithoutValue(t *testing.T) {
	_, _, err := parseMigrateDownFlags([]string{"--backup-to"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--backup-to requires a path")
}

func TestParseMigrateDownFlags_BackupToEqualsEmpty(t *testing.T) {
	_, _, err := parseMigrateDownFlags([]string{"--backup-to="})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-empty path")
}

func TestParseMigrateDownFlags_BackupToSpaceSyntax(t *testing.T) {
	force, backupTo, err := parseMigrateDownFlags([]string{"--backup-to", "/tmp/backup.sql"})
	require.NoError(t, err)
	assert.True(t, force == false)
	assert.Equal(t, "/tmp/backup.sql", backupTo)
}

func TestParseMigrateDownFlags_ForceAndBackup(t *testing.T) {
	force, backupTo, err := parseMigrateDownFlags([]string{"--force", "--backup-to=/tmp/b.sql"})
	require.NoError(t, err)
	assert.True(t, force)
	assert.Equal(t, "/tmp/b.sql", backupTo)
}

func TestPrintMigrationStatus_EmptyList(t *testing.T) {
	db := newTestSQLiteDB(t)
	m := database.NewMigrator(db.DB(), database.SQLiteDialect)
	var buf bytes.Buffer
	err := printMigrationStatus(context.Background(), m, &buf)
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "VERSION")
}

func TestPrintMigrationStatus_WithAppliedMigrations(t *testing.T) {
	db := newTestSQLiteDB(t)
	m := database.NewMigrator(db.DB(), database.SQLiteDialect)
	require.NoError(t, m.MigrateUp(context.Background()))

	var buf bytes.Buffer
	err := printMigrationStatus(context.Background(), m, &buf)
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "VERSION")
	assert.Contains(t, output, "APPLIED")
}

func TestFirstN_Short(t *testing.T) {
	assert.Equal(t, "abc", firstN("abc", 10))
}

func TestFirstN_Truncate(t *testing.T) {
	assert.Equal(t, "abcdefghij", firstN("abcdefghijklmnop", 10))
}

func TestRunMigrate_HelpSubcommand(t *testing.T) {
	db := newTestSQLiteDB(t)
	var buf bytes.Buffer
	err := runMigrate(context.Background(), db, []string{"help"}, &buf)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Usage: patreon-manager migrate")
}

func TestRunMigrate_UnknownSubcommand(t *testing.T) {
	db := newTestSQLiteDB(t)
	var buf bytes.Buffer
	err := runMigrate(context.Background(), db, []string{"badcmd"}, &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown subcommand")
}
```

Ensure required imports are present. Check that `newTestSQLiteDB` helper exists in the file (it does — used by existing tests).

- [ ] **Step 2: Run tests**

Run: `go test ./cmd/cli/... -run "TestParseMigrateDownFlags_|TestPrintMigrationStatus_|TestFirstN_|TestRunMigrate_Help|TestRunMigrate_Unknown" -v`
Expected: all PASS

- [ ] **Step 3: Commit**

```bash
git add cmd/cli/migrate_test.go
git commit -m "test(cli): add migrate coverage for parseMigrateDownFlags, printMigrationStatus, help"
```

---

## Task 7: Coverage — cmd/cli main.go

**Files:**
- Modify: `cmd/cli/main_test.go`

- [ ] **Step 1: Add main.go coverage tests**

The `main` function in `cmd/cli/main.go` handles flag parsing and command dispatch. The DI variables (`osExit`, `newDatabase`, etc.) allow test overrides. Add tests for uncovered branches — look at what the coverage report says is uncovered (lines with <100%) and add targeted tests.

Read the current test file to understand existing coverage, then append tests for:

1. `sync` deprecated command dispatch
2. `scan` command with empty providers
3. `publish` command with missing flags
4. `verify` command paths

Since `main()` calls `osExit` (a DI variable), tests can capture the exit call:

```go
func TestMain_SyncDeprecated(t *testing.T) {
	origExit := osExit
	var exitCalled bool
	osExit = func(code int) { exitCalled = true }
	defer func() { osExit = origExit }()

	origDB := newDatabase
	newDatabase = func(driver, dsn string) database.Database {
		return &mockDatabase{}
	}
	defer func() { newDatabase = origDB }()

	os.Args = []string{"patreon-manager", "sync", "--dry-run"}
	main()
	assert.True(t, exitCalled)
}

func TestMain_ValidateCommand(t *testing.T) {
	origExit := osExit
	var exitCode int
	osExit = func(code int) { exitCode = code }
	defer func() { osExit = origExit }()

	os.Args = []string{"patreon-manager", "validate"}
	main()
	assert.Equal(t, 1, exitCode)
}
```

Note: These tests may need adjustment based on what's actually uncovered. Run `go tool cover -func=/tmp/cli_cov.out | grep "main.go" | grep -v "100.0%"` to see exact uncovered lines and add targeted tests.

- [ ] **Step 2: Run tests and check coverage**

Run: `go test -race -coverprofile=/tmp/cli_cov.out ./cmd/cli/ && go tool cover -func=/tmp/cli_cov.out | grep "main.go"`
Expected: improved coverage approaching 100%

- [ ] **Step 3: Commit**

```bash
git add cmd/cli/main_test.go
git commit -m "test(cli): add main.go coverage for command dispatch"
```

---

## Task 8: Coverage — cmd/server runServer

**Files:**
- Modify: `cmd/server/coverage_gaps_test.go`

- [ ] **Step 1: Add runServer coverage tests**

Append to `cmd/server/coverage_gaps_test.go`:

```go
func TestRunServer_GracefulShutdown(t *testing.T) {
	origRunServer := runServerFn
	origSignalNotify := signalNotifyContext
	defer func() {
		runServerFn = origRunServer
		signalNotifyContext = origSignalNotify
	}()

	cfg := &config.Config{
		GinMode:           "test",
		Port:              0,
		AdminKey:          "test-key",
		WebhookHMACSecret: "test-secret",
		RateLimitRPS:      100,
		RateLimitBurst:    200,
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err := runServer(ctx, cfg, "127.0.0.1:0", slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	assert.NoError(t, err)
}

func TestRunServer_AdminKeyWarning(t *testing.T) {
	cfg := &config.Config{
		GinMode:           "test",
		Port:              0,
		AdminKey:          "",
		WebhookHMACSecret: "",
		RateLimitRPS:      100,
		RateLimitBurst:    200,
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err := runServer(ctx, cfg, "127.0.0.1:0", logger)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "ADMIN_KEY not set")
	assert.Contains(t, buf.String(), "WEBHOOK_HMAC_SECRET not set")
}

func TestRunServer_LifecycleCloseError(t *testing.T) {
	cfg := &config.Config{
		GinMode:           "test",
		Port:              0,
		AdminKey:          "test",
		WebhookHMACSecret: "test",
		RateLimitRPS:      100,
		RateLimitBurst:    200,
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err := runServer(ctx, cfg, "127.0.0.1:0", logger)
	assert.NoError(t, err)
}
```

Add `"time"` to imports if not present.

- [ ] **Step 2: Run tests**

Run: `go test ./cmd/server/... -run "TestRunServer_" -v -timeout 30s`
Expected: all PASS

- [ ] **Step 3: Commit**

```bash
git add cmd/server/coverage_gaps_test.go
git commit -m "test(server): add runServer coverage for graceful shutdown, warnings"
```

---

## Task 9: Coverage — cmd/server setupRouter

**Files:**
- Modify: `cmd/server/coverage_gaps_test.go`

- [ ] **Step 1: Add setupRouter coverage tests**

Append to `cmd/server/coverage_gaps_test.go`:

```go
func TestSetupRouter_PprofBehindAuth(t *testing.T) {
	cfg := &config.Config{
		GinMode:           "test",
		Port:              8080,
		AdminKey:          "admin-secret",
		WebhookHMACSecret: "webhook-secret",
		RateLimitRPS:      1000,
		RateLimitBurst:    2000,
	}
	router, dedup, _, _ := setupRouter(cfg, &mockMetricsCollector{}, noopOrchestrator{}, slog.Default(), nil)
	defer dedup.Close()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/debug/pprof/", nil)
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/debug/pprof/", nil)
	req.Header.Set("X-Admin-Key", "admin-secret")
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSetupRouter_DownloadRoute(t *testing.T) {
	cfg := &config.Config{
		GinMode:           "test",
		Port:              8080,
		AdminKey:          "admin-secret",
		WebhookHMACSecret: "webhook-secret",
		RateLimitRPS:      1000,
		RateLimitBurst:    2000,
	}
	router, dedup, _, _ := setupRouter(cfg, &mockMetricsCollector{}, noopOrchestrator{}, slog.Default(), nil)
	defer dedup.Close()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/download/test-content-id", nil)
	router.ServeHTTP(w, req)
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestSetupRouter_AdminAuditList(t *testing.T) {
	cfg := &config.Config{
		GinMode:           "test",
		Port:              8080,
		AdminKey:          "admin-secret",
		WebhookHMACSecret: "webhook-secret",
		RateLimitRPS:      1000,
		RateLimitBurst:    2000,
	}
	router, dedup, _, _ := setupRouter(cfg, &mockMetricsCollector{}, noopOrchestrator{}, slog.Default(), nil)
	defer dedup.Close()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/admin/audit", nil)
	req.Header.Set("X-Admin-Key", "admin-secret")
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSetupRouter_HealthEndpoint(t *testing.T) {
	cfg := &config.Config{
		GinMode:           "test",
		Port:              8080,
		AdminKey:          "admin-secret",
		WebhookHMACSecret: "webhook-secret",
		RateLimitRPS:      1000,
		RateLimitBurst:    2000,
	}
	router, dedup, _, _ := setupRouter(cfg, &mockMetricsCollector{}, noopOrchestrator{}, slog.Default(), nil)
	defer dedup.Close()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSetupRouter_DeepHealthEndpoint(t *testing.T) {
	cfg := &config.Config{
		GinMode:           "test",
		Port:              8080,
		AdminKey:          "admin-secret",
		WebhookHMACSecret: "webhook-secret",
		RateLimitRPS:      1000,
		RateLimitBurst:    2000,
	}
	router, dedup, _, _ := setupRouter(cfg, &mockMetricsCollector{}, noopOrchestrator{}, slog.Default(), nil)
	defer dedup.Close()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/admin/health/deep", nil)
	req.Header.Set("X-Admin-Key", "admin-secret")
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSetupRouter_AdminAuditListError(t *testing.T) {
	cfg := &config.Config{
		GinMode:           "test",
		Port:              8080,
		AdminKey:          "admin-secret",
		WebhookHMACSecret: "webhook-secret",
		RateLimitRPS:      1000,
		RateLimitBurst:    2000,
	}

	gin.SetMode("test")
	r := gin.New()
	handler := handlers.NewAdminHandler(slog.Default())
	handler.SetAuditStore(failingAuditStore{})

	admin := r.Group("/admin")
	admin.Use(func(c *gin.Context) { c.Next() })
	admin.GET("/audit", adminAuditList(handler))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/admin/audit", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
```

Check if `"github.com/gin-gonic/gin"` and `"github.com/milos85vasic/My-Patreon-Manager/internal/handlers"` are imported. Add if missing.

- [ ] **Step 2: Run tests**

Run: `go test ./cmd/server/... -run "TestSetupRouter_(Pprof|Download|AdminAudit|Health|Deep)" -v`
Expected: all PASS

- [ ] **Step 3: Check coverage**

Run: `go test -race -coverprofile=/tmp/server_cov.out ./cmd/server/ && go tool cover -func=/tmp/server_cov.out | grep "main.go"`
Expected: improved, approaching 100%

- [ ] **Step 4: Commit**

```bash
git add cmd/server/coverage_gaps_test.go
git commit -m "test(server): add setupRouter coverage for pprof, download, audit, health endpoints"
```

---

## Task 10: Final Coverage Verification and Push

- [ ] **Step 1: Run full envwizard tests**

Run: `go test -race ./cmd/envwizard/... -v`
Expected: all PASS

- [ ] **Step 2: Check coverage for cmd/cli and cmd/server**

Run:
```bash
go test -race -coverprofile=/tmp/cli_cov.out ./cmd/cli/ && go tool cover -func=/tmp/cli_cov.out | tail -1
go test -race -coverprofile=/tmp/server_cov.out ./cmd/server/ && go tool cover -func=/tmp/server_cov.out | tail -1
```

If below 100%, check remaining uncovered functions with:
```bash
go tool cover -func=/tmp/cli_cov.out | grep -v "100.0%"
go tool cover -func=/tmp/server_cov.out | grep -v "100.0%"
```

Then add targeted tests for remaining gaps following the same patterns.

- [ ] **Step 3: Run full test suite**

Run: `go test -race ./... 2>&1 | tail -20`
Expected: all PASS, no failures

- [ ] **Step 4: Push to all remotes**

```bash
git push github main
git push gitlab main
git push gitflic main
git push gitverse main
```
