package utils

import (
	"encoding/json"
	"fmt"
)

func ToJSON(v interface{}) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func FromJSON(data string, v interface{}) error {
	if data == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(data), v); err != nil {
		return fmt.Errorf("fromJSON: %w", err)
	}
	return nil
}
