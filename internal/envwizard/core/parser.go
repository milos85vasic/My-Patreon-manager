package core

import (
	"os"
	"strings"
)

func ParseEnvFile(content string) (map[string]string, error) {
	vars := make(map[string]string)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, "="); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			vars[key] = val
		}
	}
	return vars, nil
}

func LoadEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseEnvFile(string(data))
}

func GenerateEnvFile(vars map[string]string) string {
	var builder strings.Builder
	for key, val := range vars {
		builder.WriteString(key)
		builder.WriteString("=")
		builder.WriteString(val)
		builder.WriteString("\n")
	}
	return builder.String()
}
