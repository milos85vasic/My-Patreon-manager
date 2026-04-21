package api

import (
	"encoding/json"
	"net/http"

	"github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/core"
	"github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/definitions"
)

type Server struct {
	wizard *core.Wizard
	mux    *http.ServeMux
}

func NewServer(w *core.Wizard) *Server {
	s := &Server{
		wizard: w,
		mux:    http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/categories", s.handleCategories)
	s.mux.HandleFunc("GET /api/vars", s.handleVars)
	s.mux.HandleFunc("GET /api/vars/{name}", s.handleVarByName)
	s.mux.HandleFunc("POST /api/vars/{name}", s.handleSetValue)
	s.mux.HandleFunc("POST /api/skip/{name}", s.handleSkip)
	s.mux.HandleFunc("GET /api/wizard/state", s.handleWizardState)
	s.mux.HandleFunc("POST /api/wizard/next", s.handleNext)
	s.mux.HandleFunc("POST /api/wizard/prev", s.handlePrev)
	s.mux.HandleFunc("POST /api/save", s.handleSave)
	s.mux.HandleFunc("GET /api/profiles", s.handleListProfiles)
	s.mux.HandleFunc("POST /api/profiles", s.handleSaveProfile)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) handleCategories(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, definitions.GetCategories())
}

func (s *Server) handleVars(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, definitions.GetAll())
}

func (s *Server) handleVarByName(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	v := definitions.GetByName(name)
	if v == nil {
		writeError(w, http.StatusNotFound, "variable not found")
		return
	}
	resp := map[string]any{
		"definition": v,
		"value":      s.wizard.GetValue(name),
		"isSet":      s.wizard.IsSet(name),
		"isSkipped":  s.wizard.IsSkipped(name),
	}
	writeJSON(w, http.StatusOK, resp)
}

type setValueRequest struct {
	Value string `json:"value"`
}

func (s *Server) handleSetValue(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	v := definitions.GetByName(name)
	if v == nil {
		writeError(w, http.StatusNotFound, "variable not found")
		return
	}

	var req setValueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := definitions.ValidateValue(v, req.Value); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.wizard.SetValue(name, req.Value)
	writeJSON(w, http.StatusOK, map[string]string{"status": "set", "name": name})
}

func (s *Server) handleSkip(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	s.wizard.Skip(name)
	writeJSON(w, http.StatusOK, map[string]string{"status": "skipped", "name": name})
}

func (s *Server) handleWizardState(w http.ResponseWriter, r *http.Request) {
	completed, total := s.wizard.Progress()
	missing := s.wizard.MissingRequired()
	missingNames := make([]string, len(missing))
	for i, v := range missing {
		missingNames[i] = v.Name
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"step":             s.wizard.Step,
		"totalSteps":       total,
		"completed":        completed,
		"missingRequired":  missingNames,
		"hasErrors":        s.wizard.HasErrors(),
	})
}

func (s *Server) handleNext(w http.ResponseWriter, r *http.Request) {
	s.wizard.Next()
	writeJSON(w, http.StatusOK, map[string]any{
		"step":       s.wizard.Step,
		"currentVar": s.wizard.CurrentVar(),
	})
}

func (s *Server) handlePrev(w http.ResponseWriter, r *http.Request) {
	s.wizard.Previous()
	writeJSON(w, http.StatusOK, map[string]any{
		"step":       s.wizard.Step,
		"currentVar": s.wizard.CurrentVar(),
	})
}

func (s *Server) handleSave(w http.ResponseWriter, r *http.Request) {
	gen := core.NewGenerator(s.wizard)
	content := gen.ProduceEnvFile(false)
	writeJSON(w, http.StatusOK, map[string]string{"content": content})
}

func (s *Server) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	names, err := core.ListProfiles()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, names)
}

func (s *Server) handleSaveProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p := &core.Profile{Name: req.Name, Values: s.wizard.Values}
	if err := core.SaveProfile(p); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved", "name": req.Name})
}
