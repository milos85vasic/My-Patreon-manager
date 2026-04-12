package contract

import (
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/llm"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/patreon"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/renderer"
	"github.com/milos85vasic/My-Patreon-Manager/tests/mocks"
)

// Compile-time interface satisfaction checks.
// These ensure mocks stay in sync with the real interfaces.
// If any mock drifts, this file will fail to compile.

// Database interface
var _ database.Database = (*mocks.MockDatabase)(nil)

// Repository store
var _ database.RepositoryStore = (*mocks.MockRepositoryStore)(nil)

// Git provider
var _ git.RepositoryProvider = (*mocks.MockRepositoryProvider)(nil)

// LLM provider
var _ llm.LLMProvider = (*mocks.MockLLMProvider)(nil)

// Patreon provider
var _ patreon.Provider = (*mocks.PatreonClient)(nil)

// Format renderer
var _ renderer.FormatRenderer = (*mocks.MockFormatRenderer)(nil)

// TestInterfaceSatisfaction is a runtime test that validates the compile-time
// interface checks above actually run during test execution.
func TestInterfaceSatisfaction(t *testing.T) {
	// If this test compiles, all interface assertions above are valid.
	t.Log("All mock types satisfy their respective interfaces")
}
