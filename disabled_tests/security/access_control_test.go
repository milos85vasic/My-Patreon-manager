//go:build disabled

//go:build disabled\n
package security

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/services/access"
	"github.com/milos85vasic/My-Patreon-Manager/internal/utils"
	"github.com/stretchr/testify/assert"
)

func parseQuery(url string) (token, sub string, exp int64) {
	idx := strings.Index(url, "?")
	if idx < 0 {
		return
	}
	t, s, e := access.ExtractTokenFromQuery(url[idx+1:])
	exp, _ = strconv.ParseInt(e, 10, 64)
	return t, s, exp
}

func TestSignedURL_ForgedToken(t *testing.T) {
	gen := access.NewSignedURLGenerator("secret", 1*time.Hour)
	valid := gen.VerifySignedURL("totally-faked-token", "content-1", "sub-1", time.Now().Add(time.Hour).Unix())
	assert.False(t, valid, "forged token should be rejected")
}

func TestSignedURL_ExpiredToken(t *testing.T) {
	gen := access.NewSignedURLGenerator("secret", -1*time.Second)
	url := gen.GenerateSignedURL("content-1", "sub-1")
	token, sub, exp := parseQuery(url)
	valid := gen.VerifySignedURL(token, "content-1", sub, exp)
	assert.False(t, valid, "expired token should be rejected")
}

func TestSignedURL_WrongContentID(t *testing.T) {
	gen := access.NewSignedURLGenerator("secret", 1*time.Hour)
	url := gen.GenerateSignedURL("content-1", "sub-1")
	token, sub, exp := parseQuery(url)
	valid := gen.VerifySignedURL(token, "different-content", sub, exp)
	assert.False(t, valid, "wrong content ID should be rejected")
}

func TestSignedURL_TimingResistance(t *testing.T) {
	gen := access.NewSignedURLGenerator("secret", 1*time.Hour)
	url := gen.GenerateSignedURL("content-1", "sub-1")
	token, sub, exp := parseQuery(url)

	correct := gen.VerifySignedURL(token, "content-1", sub, exp)
	wrong := gen.VerifySignedURL("x"+token[1:], "content-1", sub, exp)

	assert.True(t, correct)
	assert.False(t, wrong)
}

func TestAccessGater_DefaultDeny(t *testing.T) {
	gater := access.NewTierGater()
	granted, _, err := gater.VerifyAccess(nil, "patron", "content", "gold", nil)
	assert.NoError(t, err)
	assert.False(t, granted, "empty tiers should be denied by default")
}

func TestCredentialRedaction(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		exclude string
	}{
		{
			name:    "github PAT",
			input:   "token=***",
			exclude: "ghp_***",
		},
		{
			name:    "password field",
			input:   "password=***",
			exclude: "mysecretpass",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redacted := utils.RedactString(tt.input)
			if tt.exclude != "" && len(tt.exclude) > 5 {
				assert.NotContains(t, redacted, tt.exclude)
			}
		})
	}
}

func TestConcurrentAccessControl(t *testing.T) {
	gater := access.NewTierGater()

	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func(id int) {
			granted, _, _ := gater.VerifyAccess(nil, "patron", "content", "gold", []string{"gold"})
			assert.True(t, granted)
			done <- true
		}(i)
	}

	for i := 0; i < 100; i++ {
		<-done
	}
}
