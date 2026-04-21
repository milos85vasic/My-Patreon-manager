package tui

import (
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/core"
)

var tuiTestVars = []*core.EnvVar{
	{Name: "PORT", Description: "HTTP port", Required: true, Validation: core.ValidationPort, Default: "8080"},
	{Name: "ADMIN_KEY", Description: "Admin key", Required: true, Secret: true},
	{Name: "GITHUB_TOKEN", Description: "GitHub token", Required: false, Secret: true},
	{Name: "DEBUG", Description: "Debug mode", Required: false, Default: "false", Validation: core.ValidationBoolean},
}

func TestNewTUI(t *testing.T) {
	w := core.NewWizard(tuiTestVars)
	tui := New(w)
	if tui == nil {
		t.Fatal("expected non-nil TUI")
	}
	if tui.CurrentScreen() != ScreenWelcome {
		t.Fatalf("expected ScreenWelcome, got %v", tui.CurrentScreen())
	}
}

func TestNewTUI_NilWizardPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic with nil wizard")
		}
	}()
	New(nil)
}

func TestTUI_SwitchToCategories(t *testing.T) {
	w := core.NewWizard(tuiTestVars)
	tui := New(w)
	tui.SwitchScreenForTest(ScreenCategories)
	if tui.CurrentScreen() != ScreenCategories {
		t.Fatalf("expected ScreenCategories, got %v", tui.CurrentScreen())
	}
}

func TestTUI_SwitchToSummary(t *testing.T) {
	w := core.NewWizard(tuiTestVars)
	tui := New(w)
	tui.SwitchScreenForTest(ScreenSummary)
	if tui.CurrentScreen() != ScreenSummary {
		t.Fatalf("expected ScreenSummary, got %v", tui.CurrentScreen())
	}
}

func TestTUI_SwitchToSave(t *testing.T) {
	w := core.NewWizard(tuiTestVars)
	tui := New(w)
	tui.SwitchScreenForTest(ScreenSave)
	if tui.CurrentScreen() != ScreenSave {
		t.Fatalf("expected ScreenSave, got %v", tui.CurrentScreen())
	}
}

func TestTUI_SwitchToProfile(t *testing.T) {
	w := core.NewWizard(tuiTestVars)
	tui := New(w)
	tui.SwitchScreenForTest(ScreenProfileSave)
	if tui.CurrentScreen() != ScreenProfileSave {
		t.Fatalf("expected ScreenProfileSave, got %v", tui.CurrentScreen())
	}
}

func TestTUI_SetValueViaWizard(t *testing.T) {
	w := core.NewWizard(tuiTestVars)
	tui := New(w)
	tui.SwitchScreenForTest(ScreenCategories)

	tui.wizard.SetValue("PORT", "9090")
	if !tui.wizard.IsSet("PORT") {
		t.Fatal("expected PORT to be set")
	}
	if tui.wizard.GetValue("PORT") != "9090" {
		t.Fatalf("expected PORT=9090, got %s", tui.wizard.GetValue("PORT"))
	}
}

func TestTUI_WizardProgress(t *testing.T) {
	w := core.NewWizard(tuiTestVars)
	tui := New(w)

	completed, total := tui.wizard.Progress()
	if completed != 0 {
		t.Fatalf("expected 0 completed, got %d", completed)
	}
	if total != len(tuiTestVars) {
		t.Fatalf("expected %d total, got %d", len(tuiTestVars), total)
	}

	tui.wizard.SetValue("PORT", "8080")
	completed, total = tui.wizard.Progress()
	if completed != 1 {
		t.Fatalf("expected 1 completed, got %d", completed)
	}
	if total != len(tuiTestVars) {
		t.Fatalf("expected %d total, got %d", len(tuiTestVars), total)
	}
}

func TestTUI_RefreshCategories_PopulatesTable(t *testing.T) {
	vars := []*core.EnvVar{
		{Name: "PORT", Description: "HTTP port", Category: &core.Category{ID: "server", Name: "Server", Order: 1}, Required: true, Default: "8080", Validation: core.ValidationPort},
		{Name: "DEBUG", Description: "Debug mode", Category: &core.Category{ID: "server", Name: "Server", Order: 1}, Required: false, Default: "false", Validation: core.ValidationBoolean},
		{Name: "ADMIN_KEY", Description: "Admin key", Category: &core.Category{ID: "security", Name: "Security", Order: 2}, Required: true, Secret: true},
	}
	w := core.NewWizard(vars)
	tui := New(w)

	tui.wizard.SetValue("PORT", "3000")
	tui.SwitchScreenForTest(ScreenCategories)

	rowCount := tui.categoriesTable.GetRowCount()
	if rowCount < 2 {
		t.Fatalf("expected at least header + 1 data row, got %d rows", rowCount)
	}

	cell := tui.categoriesTable.GetCell(1, 0)
	if cell == nil {
		t.Fatal("expected non-nil cell at row 1, col 0")
	}
	text := cell.Text
	if text == "" {
		t.Fatal("expected category name in cell, got empty string")
	}

	found := false
	for r := 1; r < rowCount; r++ {
		c := tui.categoriesTable.GetCell(r, 0)
		if c != nil && c.Text == "Server" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected to find 'Server' category in table")
	}

	progressCell := tui.categoriesTable.GetCell(1, 2)
	if progressCell == nil {
		t.Fatal("expected non-nil progress cell")
	}
	if progressCell.Text != "1/2" {
		t.Fatalf("expected progress '1/2' for Server (only PORT set), got '%s'", progressCell.Text)
	}
}
