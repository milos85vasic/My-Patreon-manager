package core

type Wizard struct {
	ProfileName string
	Step        int
	History     []int
	Values      map[string]string
	Skipped     map[string]bool
	Errors      map[string]error
	Modified    map[string]bool
	Vars        []*EnvVar
}

func NewWizard(vars []*EnvVar) *Wizard {
	return &Wizard{
		Values:   make(map[string]string),
		Skipped:  make(map[string]bool),
		Errors:   make(map[string]error),
		Modified: make(map[string]bool),
		History:  []int{0},
		Vars:     vars,
	}
}

func NewWizardFromEnvFile(vars []*EnvVar, path string) (*Wizard, error) {
	loaded, err := LoadEnvFile(path)
	if err != nil {
		return nil, err
	}
	w := NewWizard(vars)
	for k, v := range loaded {
		w.Values[k] = v
		w.Modified[k] = true
	}
	return w, nil
}

func (w *Wizard) TotalSteps() int {
	return len(w.Vars)
}

func (w *Wizard) CurrentVar() *EnvVar {
	if w.Step >= 0 && w.Step < len(w.Vars) {
		return w.Vars[w.Step]
	}
	return nil
}

func (w *Wizard) Next() bool {
	if w.Step < w.TotalSteps()-1 {
		w.Step++
		w.History = append(w.History, w.Step)
		return true
	}
	return false
}

func (w *Wizard) Previous() bool {
	if w.Step > 0 {
		w.Step--
		w.History = append(w.History, w.Step)
		return true
	}
	return false
}

func (w *Wizard) GoToStep(step int) bool {
	if step >= 0 && step < w.TotalSteps() {
		w.Step = step
		w.History = append(w.History, w.Step)
		return true
	}
	return false
}

func (w *Wizard) SetValue(key, value string) {
	w.Values[key] = value
	w.Modified[key] = true
	delete(w.Errors, key)
	delete(w.Skipped, key)
}

func (w *Wizard) GetValue(key string) string {
	return w.Values[key]
}

func (w *Wizard) Skip(key string) {
	w.Skipped[key] = true
	delete(w.Values, key)
	delete(w.Errors, key)
}

func (w *Wizard) IsSkipped(key string) bool {
	return w.Skipped[key]
}

func (w *Wizard) IsSet(key string) bool {
	_, ok := w.Values[key]
	return ok
}

func (w *Wizard) SetError(key string, err error) {
	w.Errors[key] = err
}

func (w *Wizard) GetError(key string) error {
	return w.Errors[key]
}

func (w *Wizard) HasErrors() bool {
	return len(w.Errors) > 0
}

func (w *Wizard) MissingRequired() []*EnvVar {
	var missing []*EnvVar
	for _, v := range w.Vars {
		if v.Required && !w.IsSet(v.Name) && !w.IsSkipped(v.Name) {
			missing = append(missing, v)
		}
	}
	return missing
}

func (w *Wizard) Progress() (completed, total int) {
	total = w.TotalSteps()
	for _, v := range w.Vars {
		if w.IsSet(v.Name) || w.IsSkipped(v.Name) {
			completed++
		}
	}
	return
}
