package definitions_test

import (
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/definitions"
	"github.com/stretchr/testify/assert"
)

func TestGetAll_Count(t *testing.T) {
	vars := definitions.GetAll()
	assert.Greater(t, len(vars), 50, "should have 50+ env vars defined")
}

func TestGetAll_AllCategorized(t *testing.T) {
	for _, v := range definitions.GetAll() {
		assert.NotNil(t, v.Category, "var %s should have a category", v.Name)
		assert.NotEmpty(t, v.Category.ID, "var %s category should have ID", v.Name)
	}
}

func TestGetAll_NoDuplicateNames(t *testing.T) {
	seen := make(map[string]bool)
	for _, v := range definitions.GetAll() {
		assert.False(t, seen[v.Name], "duplicate var name: %s", v.Name)
		seen[v.Name] = true
	}
}

func TestGetByName_Found(t *testing.T) {
	v := definitions.GetByName("PORT")
	assert.NotNil(t, v)
	assert.Equal(t, "PORT", v.Name)
	assert.True(t, v.Required)
}

func TestGetByName_NotFound(t *testing.T) {
	v := definitions.GetByName("NONEXISTENT_VAR")
	assert.Nil(t, v)
}

func TestGetByCategory(t *testing.T) {
	serverVars := definitions.GetByCategory("server")
	assert.NotEmpty(t, serverVars)
	for _, v := range serverVars {
		assert.Equal(t, "server", v.Category.ID)
	}
}

func TestGetRequired(t *testing.T) {
	required := definitions.GetRequired()
	assert.NotEmpty(t, required)
	for _, v := range required {
		assert.True(t, v.Required, "var %s should be required", v.Name)
	}
}

func TestGetSecrets(t *testing.T) {
	secrets := definitions.GetSecrets()
	assert.NotEmpty(t, secrets)
	for _, v := range secrets {
		assert.True(t, v.Secret, "var %s should be secret", v.Name)
	}
}

func TestGetCategories_Count(t *testing.T) {
	cats := definitions.GetCategories()
	assert.GreaterOrEqual(t, len(cats), 16, "should have 16+ categories")
}

func TestGetCategoryByID(t *testing.T) {
	cat := definitions.GetCategoryByID("server")
	assert.NotNil(t, cat)
	assert.Equal(t, "server", cat.ID)
}

func TestGetCategoryByID_NotFound(t *testing.T) {
	cat := definitions.GetCategoryByID("nonexistent")
	assert.Nil(t, cat)
}

func TestRequiredVarsHaveDescriptions(t *testing.T) {
	for _, v := range definitions.GetRequired() {
		assert.NotEmpty(t, v.Description, "required var %s needs description", v.Name)
	}
}

func TestSecretVarsMarked(t *testing.T) {
	secretNames := []string{"PATREON_CLIENT_SECRET", "PATREON_ACCESS_TOKEN", "HMAC_SECRET", "ADMIN_KEY"}
	for _, name := range secretNames {
		v := definitions.GetByName(name)
		assert.NotNil(t, v, "var %s not found", name)
		assert.True(t, v.Secret, "var %s should be marked secret", name)
	}
}

func TestGeneratableVars(t *testing.T) {
	genVars := []string{"HMAC_SECRET", "ADMIN_KEY", "REVIEWER_KEY", "WEBHOOK_HMAC_SECRET"}
	for _, name := range genVars {
		v := definitions.GetByName(name)
		assert.NotNil(t, v, "var %s not found", name)
		assert.True(t, v.CanGenerate, "var %s should be generatable", name)
	}
}
