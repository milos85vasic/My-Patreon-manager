package middleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVerifyHMACSignature_EmptySecret(t *testing.T) {
	assert.False(t, ValidateGitHubSignature([]byte("body"), "sha256=abc", ""))
}

func TestVerifyHMACSignature_EmptyHeader(t *testing.T) {
	assert.False(t, ValidateGitHubSignature([]byte("body"), "", "secret"))
}

func TestValidateGenericSignature_EmptySecret(t *testing.T) {
	assert.False(t, ValidateGenericSignature([]byte("body"), "sha256=abc", ""))
}

func TestValidateGenericSignature_EmptyHeader(t *testing.T) {
	assert.False(t, ValidateGenericSignature([]byte("body"), "", "secret"))
}
