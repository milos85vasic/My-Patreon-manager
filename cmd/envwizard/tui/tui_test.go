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
