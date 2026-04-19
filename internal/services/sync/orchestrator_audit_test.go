package sync

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/services/audit"
	"github.com/milos85vasic/My-Patreon-Manager/tests/mocks"
	"github.com/stretchr/testify/assert"
)

func TestOrchestrator_SetAuditStore(t *testing.T) {
	orc := NewOrchestrator(&mocks.MockDatabase{}, nil, &mocks.PatreonClient{}, nil, nil, slog.Default(), nil)
	custom := audit.NewRingStore(8)
	orc.SetAuditStore(custom)
	assert.Same(t, custom, orc.AuditStore())

	orc.SetAuditStore(nil)
	assert.NotNil(t, orc.AuditStore())
	assert.NotSame(t, custom, orc.AuditStore())
}

func TestOrchestrator_ShortErr(t *testing.T) {
	assert.Equal(t, "", shortErr(nil))
	assert.Equal(t, "boom", shortErr(errors.New("boom")))
	// Long string is truncated to 96 chars.
	long := make([]byte, 200)
	for i := range long {
		long[i] = 'a'
	}
	assert.Equal(t, 96, len(shortErr(errors.New(string(long)))))
	// Token is redacted.
	out := shortErr(errors.New("Bearer token=xyz failed"))
	assert.NotContains(t, out, "token")
	assert.Contains(t, out, "***")
}
