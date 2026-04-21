package definitions_test

import (
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/core"
	"github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/definitions"
	"github.com/stretchr/testify/assert"
)

func TestValidatePort(t *testing.T) {
	v := &core.EnvVar{Name: "PORT", Validation: core.ValidationPort}
	assert.NoError(t, definitions.ValidateValue(v, "8080"))
	assert.NoError(t, definitions.ValidateValue(v, "1"))
	assert.NoError(t, definitions.ValidateValue(v, "65535"))
	assert.Error(t, definitions.ValidateValue(v, "0"))
	assert.Error(t, definitions.ValidateValue(v, "70000"))
	assert.Error(t, definitions.ValidateValue(v, "abc"))
}

func TestValidateURL(t *testing.T) {
	v := &core.EnvVar{Name: "URL", Validation: core.ValidationURL}
	assert.NoError(t, definitions.ValidateValue(v, "http://localhost:8080"))
	assert.NoError(t, definitions.ValidateValue(v, "https://example.com"))
	assert.Error(t, definitions.ValidateValue(v, "not-a-url"))
	assert.Error(t, definitions.ValidateValue(v, "localhost:8080"))
}

func TestValidateBoolean(t *testing.T) {
	v := &core.EnvVar{Name: "FLAG", Validation: core.ValidationBoolean}
	assert.NoError(t, definitions.ValidateValue(v, "true"))
	assert.NoError(t, definitions.ValidateValue(v, "false"))
	assert.NoError(t, definitions.ValidateValue(v, "1"))
	assert.NoError(t, definitions.ValidateValue(v, "0"))
	assert.NoError(t, definitions.ValidateValue(v, "yes"))
	assert.NoError(t, definitions.ValidateValue(v, "no"))
	assert.Error(t, definitions.ValidateValue(v, "maybe"))
}

func TestValidateNumber(t *testing.T) {
	v := &core.EnvVar{Name: "NUM", Validation: core.ValidationNumber}
	assert.NoError(t, definitions.ValidateValue(v, "100"))
	assert.NoError(t, definitions.ValidateValue(v, "0"))
	assert.NoError(t, definitions.ValidateValue(v, "-5"))
	assert.Error(t, definitions.ValidateValue(v, "abc"))
	assert.Error(t, definitions.ValidateValue(v, "12.5"))
}

func TestValidateCustom(t *testing.T) {
	v := &core.EnvVar{Name: "MODE", Validation: core.ValidationCustom, ValidationRule: "^(debug|release|test)$"}
	assert.NoError(t, definitions.ValidateValue(v, "debug"))
	assert.NoError(t, definitions.ValidateValue(v, "release"))
	assert.Error(t, definitions.ValidateValue(v, "production"))
}

func TestValidateCustom_EmptyRule(t *testing.T) {
	v := &core.EnvVar{Name: "FREE", Validation: core.ValidationCustom, ValidationRule: ""}
	assert.NoError(t, definitions.ValidateValue(v, "anything"))
}

func TestValidateRequired(t *testing.T) {
	v := &core.EnvVar{Name: "KEY", Required: true}
	assert.Error(t, definitions.ValidateValue(v, ""))
	assert.NoError(t, definitions.ValidateValue(v, "some-value"))
}

func TestValidateOptionalEmpty(t *testing.T) {
	v := &core.EnvVar{Name: "OPT", Required: false}
	assert.NoError(t, definitions.ValidateValue(v, ""))
}

func TestValidateToken(t *testing.T) {
	v := &core.EnvVar{Name: "TOKEN", Validation: core.ValidationToken}
	assert.NoError(t, definitions.ValidateValue(v, "longtoken123"))
	assert.Error(t, definitions.ValidateValue(v, "short"))
}

func TestValidateAll(t *testing.T) {
	vars := []*core.EnvVar{
		{Name: "PORT", Validation: core.ValidationPort, Required: true},
		{Name: "DEBUG", Validation: core.ValidationBoolean, Required: false},
	}
	values := map[string]string{"PORT": "8080", "DEBUG": "true"}
	errs := definitions.ValidateAll(vars, values)
	assert.Empty(t, errs)
}

func TestValidateAll_WithErrors(t *testing.T) {
	vars := []*core.EnvVar{
		{Name: "PORT", Validation: core.ValidationPort, Required: true},
		{Name: "DEBUG", Validation: core.ValidationBoolean, Required: false},
	}
	values := map[string]string{"PORT": "abc", "DEBUG": "maybe"}
	errs := definitions.ValidateAll(vars, values)
	assert.Len(t, errs, 2)
}
