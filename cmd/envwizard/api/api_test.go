package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	envapi "github.com/milos85vasic/My-Patreon-Manager/cmd/envwizard/api"
	"github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/core"
	"github.com/stretchr/testify/assert"
)

var apiTestVars = []*core.EnvVar{
	{Name: "PORT", Description: "HTTP port", Required: true, Validation: core.ValidationPort, Default: "8080"},
	{Name: "ADMIN_KEY", Description: "Admin key", Required: true, Secret: true},
}

func newTestServer(t *testing.T) (*envapi.Server, *core.Wizard) {
	t.Helper()
	w := core.NewWizard(apiTestVars)
	s := envapi.NewServer(w)
	return s, w
}

func TestAPI_GetCategories(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/categories", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAPI_GetVars(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/vars", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAPI_GetVarByName(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/vars/PORT", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	assert.NotNil(t, resp["definition"])
	assert.Equal(t, false, resp["isSet"])
}

func TestAPI_GetVarByName_NotFound(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/vars/NONEXISTENT", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAPI_SetValue(t *testing.T) {
	s, _ := newTestServer(t)
	body, _ := json.Marshal(map[string]string{"value": "3000"})
	req := httptest.NewRequest("POST", "/api/vars/PORT", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAPI_SetValue_NotFound(t *testing.T) {
	s, _ := newTestServer(t)
	body, _ := json.Marshal(map[string]string{"value": "x"})
	req := httptest.NewRequest("POST", "/api/vars/NOPE", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAPI_SetValue_InvalidBody(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest("POST", "/api/vars/PORT", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAPI_SetValue_InvalidValue(t *testing.T) {
	s, _ := newTestServer(t)
	body, _ := json.Marshal(map[string]string{"value": "abc"})
	req := httptest.NewRequest("POST", "/api/vars/PORT", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAPI_Skip(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest("POST", "/api/skip/PORT", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAPI_WizardState(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/wizard/state", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	assert.Equal(t, float64(0), resp["step"])
}

func TestAPI_Navigation(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest("POST", "/api/wizard/next", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	req = httptest.NewRequest("POST", "/api/wizard/prev", nil)
	w = httptest.NewRecorder()
	s.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAPI_Save(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest("POST", "/api/save", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	assert.NotEmpty(t, resp["content"])
}

func TestAPI_ListProfiles(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/profiles", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAPI_SaveProfile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s, _ := newTestServer(t)
	body, _ := json.Marshal(map[string]string{"name": "test-profile"})
	req := httptest.NewRequest("POST", "/api/profiles", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAPI_SaveProfile_InvalidBody(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest("POST", "/api/profiles", bytes.NewReader([]byte("bad json")))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
