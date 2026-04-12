package utils

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVerifySignedURL_InvalidExpiration(t *testing.T) {
	// Token with non-numeric expiration field
	_, _, err := VerifySignedURL("sig:notanumber:cid:sid", "secret")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid expiration")
}

func TestToJSON_MarshalError(t *testing.T) {
	// json.Marshal fails on values like math.Inf
	_, err := ToJSON(math.Inf(1))
	assert.Error(t, err)
}

func TestFromJSON_InvalidJSON(t *testing.T) {
	var result map[string]interface{}
	err := FromJSON("{invalid json}", &result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "fromJSON")
}
