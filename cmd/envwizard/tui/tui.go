package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/core"
	"github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/definitions"
	"github.com/rivo/tview"
)

type Screen int

const (
	ScreenWelcome     Screen = iota
	ScreenCategories
	ScreenVariable
	ScreenSummary
	ScreenSave
	ScreenProfileSave
)

type TUI struct {
	app           *tview.Application
	pages         *tview.Pages
	wizard        *core.Wizard
	currentScreen Screen
	envPath       string
	profileName   string

	welcomeFlex     *tview.Flex
	categoriesTable *tview.Table
	variableForm    *tview.Form
	summaryTable    *tview.Table
	saveForm        *tview.Form
	profileForm     *tview.Form

	statusBar    *tview.TextView
	helpBar      *tview.TextView
	currentCatID string
}

func New(w *core.Wizard) *TUI {
	if w == nil {
		panic("tui: wizard must not be nil")
	}
	t := &TUI{
		app:      tview.NewApplication(),
		pages:    tview.NewPages(),
		wizard:   w,
		envPath:  ".env",
		statusBar: tview.NewTextView().SetDynamicColors(true),
		helpBar:   tview.NewTextView().SetDynamicColors(true),
	}
	t.buildWelcome()
	t.buildCategories()
	t.buildVariable()
	t.buildSummary()
	t.buildSave()
	t.buildProfileSave()

	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(t.pages, 0, 1, true).
		AddItem(t.statusBar, 1, 0, false).
		AddItem(t.helpBar, 1, 0, false)

	t.app.SetRoot(root, true)
	t.app.SetInputCapture(t.globalInputCapture)
	t.switchScreen(ScreenWelcome)
	return t
}

func (t *TUI) Run() error {
	return t.app.Run()
}

func (t *TUI) CurrentScreen() Screen {
	return t.currentScreen
}

func (t *TUI) SwitchScreenForTest(s Screen) {
	t.switchScreen(s)
}

func (t *TUI) globalInputCapture(event *tcell.EventKey) *tcell.EventKey {
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

func (t *TUI) switchScreen(s Screen) {
	t.currentScreen = s
	t.updateStatus()
	t.updateHelp()

	switch s {
	case ScreenWelcome:
		t.pages.SwitchToPage("welcome")
		t.app.SetFocus(t.welcomeFlex)
	case ScreenCategories:
		t.refreshCategories()
		t.pages.SwitchToPage("categories")
		t.app.SetFocus(t.categoriesTable)
	case ScreenVariable:
		t.refreshVariable()
		t.pages.SwitchToPage("variable")
		t.app.SetFocus(t.variableForm)
	case ScreenSummary:
		t.refreshSummary()
		t.pages.SwitchToPage("summary")
		t.app.SetFocus(t.summaryTable)
	case ScreenSave:
		t.pages.SwitchToPage("save")
		t.app.SetFocus(t.saveForm)
	case ScreenProfileSave:
		t.pages.SwitchToPage("profile")
		t.app.SetFocus(t.profileForm)
	}
}

func (t *TUI) updateStatus() {
	completed, total := t.wizard.Progress()
	var screenName string
	switch t.currentScreen {
	case ScreenWelcome:
		screenName = "Welcome"
	case ScreenCategories:
		screenName = "Categories"
	case ScreenVariable:
		screenName = "Variable Editor"
	case ScreenSummary:
		screenName = "Summary"
	case ScreenSave:
		screenName = "Save"
	case ScreenProfileSave:
		screenName = "Profile Save"
	}
	fmt.Fprintf(t.statusBar, " [green]%s[-] | Progress: [yellow]%d/%d[-]", screenName, completed, total)
}

func (t *TUI) updateHelp() {
	t.helpBar.SetText(" [blue]Ctrl+Q[-] Quit  [blue]Ctrl+S[-] Save  [blue]Ctrl+R[-] Summary")
}

func (t *TUI) buildWelcome() {
	title := tview.NewTextView().SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)
	fmt.Fprintf(title, "\n[green::b]My Patreon Manager — Environment Wizard[-:-:-]\n\n")
	fmt.Fprintf(title, "This wizard will guide you through configuring your environment variables.\n")
	fmt.Fprintf(title, "You can load an existing profile or start fresh.\n\n")

	profileDropdown := tview.NewDropDown().SetLabel("Profile: ").SetFieldWidth(30)
	profiles, err := core.ListProfiles()
	if err == nil && len(profiles) > 0 {
		options := append([]string{"(none)"}, profiles...)
		profileDropdown.SetOptions(options, func(text string, index int) {
			if text != "(none)" {
				p, err := core.LoadProfile(text)
				if err == nil {
					t.profileName = p.Name
					for k, v := range p.Values {
						t.wizard.SetValue(k, v)
					}
				}
			}
		})
	}

	envPathInput := tview.NewInputField().SetLabel(".env path: ").
		SetFieldWidth(40).
		SetText(t.envPath).
		SetChangedFunc(func(text string) {
			t.envPath = text
		})

	startBtn := tview.NewButton("Start").SetSelectedFunc(func() {
		t.switchScreen(ScreenCategories)
	})
	quitBtn := tview.NewButton("Quit").SetSelectedFunc(func() {
		t.app.Stop()
	})

	buttonRow := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(startBtn, 10, 0, true).
		AddItem(nil, 2, 0, false).
		AddItem(quitBtn, 10, 0, true)

	t.welcomeFlex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(title, 7, 0, false).
		AddItem(profileDropdown, 1, 0, true).
		AddItem(nil, 1, 0, false).
		AddItem(envPathInput, 1, 0, true).
		AddItem(nil, 1, 0, false).
		AddItem(buttonRow, 1, 0, true)

	t.pages.AddPage("welcome", t.welcomeFlex, true, true)
}

func (t *TUI) buildCategories() {
	t.categoriesTable = tview.NewTable().SetBorders(true).
		SetSelectable(true, false).
		SetSelectedFunc(func(row, col int) {
			if row == 0 {
				return
			}
			cats := definitions.GetCategories()
			if row-1 < len(cats) {
				cat := cats[row-1]
				t.currentCatID = cat.ID
				t.wizard.GoToStep(0)
				for i, v := range t.wizard.Vars {
					if v.Category != nil && v.Category.ID == cat.ID {
						t.wizard.GoToStep(i)
						break
					}
				}
				t.switchScreen(ScreenVariable)
			}
		})

	t.categoriesTable.SetFixed(1, 0).SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEscape {
			t.switchScreen(ScreenWelcome)
		}
	})

	t.pages.AddPage("categories", t.categoriesTable, true, true)
}

func (t *TUI) refreshCategories() {
	t.categoriesTable.Clear()
	header := []string{"Category", "Description", "Progress"}
	for i, h := range header {
		t.categoriesTable.SetCell(0, i, tview.NewTableCell(h).
			SetTextColor(tcell.ColorYellow).SetSelectable(false))
	}

	cats := definitions.GetCategories()
	for i, cat := range cats {
		t.categoriesTable.SetCell(i+1, 0, tview.NewTableCell(cat.Name))
		t.categoriesTable.SetCell(i+1, 1, tview.NewTableCell(cat.Description))

		var set, total int
		for _, v := range t.wizard.Vars {
			if v.Category != nil && v.Category.ID == cat.ID {
				total++
				if t.wizard.IsSet(v.Name) || t.wizard.IsSkipped(v.Name) {
					set++
				}
			}
		}
		progress := fmt.Sprintf("%d/%d", set, total)
		t.categoriesTable.SetCell(i+1, 2, tview.NewTableCell(progress))
	}
}

func (t *TUI) buildVariable() {
	t.variableForm = tview.NewForm().SetButtonsAlign(tview.AlignCenter)
	t.pages.AddPage("variable", t.variableForm, true, true)
}

func (t *TUI) refreshVariable() {
	t.variableForm.Clear(false)

	v := t.wizard.CurrentVar()
	if v == nil {
		return
	}

	label := fmt.Sprintf("[green]%s[-]", v.Name)
	if v.Required {
		label += " [red](required)[-]"
	}
	desc := v.Description
	if v.URL != "" {
		desc += fmt.Sprintf("\n  %s", v.URL)
	}
	if v.Example != "" {
		desc += fmt.Sprintf("\n  Example: %s", v.Example)
	}

	t.variableForm.SetTitle(label).SetTitleColor(tcell.ColorGreen)

	currentVal := t.wizard.GetValue(v.Name)
	if currentVal == "" && v.Default != "" {
		currentVal = v.Default
	}

	input := tview.NewInputField().SetLabel("Value: ").
		SetFieldWidth(50).
		SetText(currentVal)

	if v.Secret {
		input.SetMaskCharacter('*')
	}

	input.SetChangedFunc(func(text string) {
		if v.Validation != "" {
			if err := definitions.ValidateValue(v, text); err != nil {
				t.statusBar.SetText(fmt.Sprintf(" [red]Validation: %s[-]", err.Error()))
			} else {
				t.updateStatus()
			}
		}
	})

	t.variableForm.AddFormItem(input)

	t.variableForm.AddButton("Next", func() {
		val := input.GetText()
		if v.Validation != "" {
			if err := definitions.ValidateValue(v, val); err != nil {
				t.wizard.SetError(v.Name, err)
				t.statusBar.SetText(fmt.Sprintf(" [red]Validation: %s[-]", err.Error()))
				return
			}
		}
		t.wizard.SetValue(v.Name, val)
		if !t.wizard.Next() {
			t.switchScreen(ScreenCategories)
			return
		}
		cv := t.wizard.CurrentVar()
		if cv != nil && cv.Category != nil && cv.Category.ID != t.currentCatID {
			t.switchScreen(ScreenCategories)
			return
		}
		t.refreshVariable()
	})

	t.variableForm.AddButton("Skip", func() {
		if v.Required {
			t.statusBar.SetText(" [red]Cannot skip required variable[-]")
			return
		}
		t.wizard.Skip(v.Name)
		if !t.wizard.Next() {
			t.switchScreen(ScreenCategories)
			return
		}
		cv := t.wizard.CurrentVar()
		if cv != nil && cv.Category != nil && cv.Category.ID != t.currentCatID {
			t.switchScreen(ScreenCategories)
			return
		}
		t.refreshVariable()
	})

	t.variableForm.AddButton("Previous", func() {
		t.wizard.Previous()
		t.refreshVariable()
	})

	t.variableForm.AddButton("Back to Categories", func() {
		t.switchScreen(ScreenCategories)
	})
}

func (t *TUI) buildSummary() {
	t.summaryTable = tview.NewTable().SetBorders(true).SetSelectable(true, false)
	t.summaryTable.SetFixed(1, 0).SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEscape {
			t.switchScreen(ScreenCategories)
		}
	})

	t.pages.AddPage("summary", t.summaryTable, true, true)
}

func (t *TUI) refreshSummary() {
	t.summaryTable.Clear()
	headers := []string{"Variable", "Value", "Status"}
	for i, h := range headers {
		t.summaryTable.SetCell(0, i, tview.NewTableCell(h).
			SetTextColor(tcell.ColorYellow).SetSelectable(false))
	}

	row := 1
	for _, v := range t.wizard.Vars {
		t.summaryTable.SetCell(row, 0, tview.NewTableCell(v.Name))

		val := t.wizard.GetValue(v.Name)
		if val == "" && v.Default != "" {
			val = v.Default
		}
		if v.Secret && val != "" {
			val = strings.Repeat("*", len(val))
		}
		displayVal := val
		if displayVal == "" {
			displayVal = "(empty)"
		}
		t.summaryTable.SetCell(row, 1, tview.NewTableCell(displayVal))

		var status string
		var color tcell.Color
		if t.wizard.IsSet(v.Name) {
			status = "✓ Set"
			color = tcell.ColorGreen
		} else if t.wizard.IsSkipped(v.Name) {
			status = "⊘ Skipped"
			color = tcell.ColorYellow
		} else if v.Required {
			status = "✗ Missing (required)"
			color = tcell.ColorRed
		} else {
			status = "○ Optional"
			color = tcell.ColorWhite
		}
		t.summaryTable.SetCell(row, 2, tview.NewTableCell(status).SetTextColor(color))
		row++
	}
}

func (t *TUI) buildSave() {
	t.saveForm = tview.NewForm().SetButtonsAlign(tview.AlignCenter)

	pathInput := tview.NewInputField().SetLabel("File path: ").
		SetFieldWidth(50).
		SetText(t.envPath)

	t.saveForm.AddFormItem(pathInput)

	t.saveForm.AddButton("Save", func() {
		t.envPath = pathInput.GetText()
		gen := core.NewGenerator(t.wizard)
		if err := gen.SaveToPath(t.envPath, false); err != nil {
			t.statusBar.SetText(fmt.Sprintf(" [red]Error: %s[-]", err.Error()))
			return
		}
		t.statusBar.SetText(fmt.Sprintf(" [green]Saved to %s[-]", t.envPath))
	})

	t.saveForm.AddButton("Back", func() {
		t.switchScreen(ScreenCategories)
	})

	t.pages.AddPage("save", t.saveForm, true, true)
}

func (t *TUI) buildProfileSave() {
	t.profileForm = tview.NewForm().SetButtonsAlign(tview.AlignCenter)

	nameInput := tview.NewInputField().SetLabel("Profile name: ").
		SetFieldWidth(40).
		SetText(t.profileName)

	t.profileForm.AddFormItem(nameInput)

	t.profileForm.AddButton("Save Profile", func() {
		name := nameInput.GetText()
		if name == "" {
			t.statusBar.SetText(" [red]Profile name is required[-]")
			return
		}
		p := &core.Profile{
			Name:   name,
			Values: t.wizard.Values,
		}
		if err := core.SaveProfile(p); err != nil {
			t.statusBar.SetText(fmt.Sprintf(" [red]Error: %s[-]", err.Error()))
			return
		}
		t.profileName = name
		t.statusBar.SetText(fmt.Sprintf(" [green]Profile '%s' saved[-]", name))
	})

	t.profileForm.AddButton("Skip & Exit", func() {
		t.app.Stop()
	})

	t.profileForm.AddButton("Back", func() {
		t.switchScreen(ScreenSave)
	})

	t.pages.AddPage("profile", t.profileForm, true, true)
}
