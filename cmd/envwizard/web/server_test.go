package web_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/cmd/envwizard/web"
	"github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/core"
	"github.com/stretchr/testify/assert"
)

func TestWeb_IndexPage(t *testing.T) {
	vars := []*core.EnvVar{
		{Name: "PORT", Description: "HTTP port", Required: true, Default: "8080"},
	}
	w := core.NewWizard(vars)
	srv := web.NewServer(w)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "EnvWizard")
}

func TestWeb_APIProxy(t *testing.T) {
	vars := []*core.EnvVar{
		{Name: "PORT", Description: "HTTP port", Required: true, Default: "8080"},
	}
	w := core.NewWizard(vars)
	srv := web.NewServer(w)

	req := httptest.NewRequest("GET", "/api/wizard/state", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestWeb_StaticAssets(t *testing.T) {
	vars := []*core.EnvVar{
		{Name: "PORT", Description: "HTTP port", Required: true},
	}
	w := core.NewWizard(vars)
	srv := web.NewServer(w)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "EnvWizard")
}
