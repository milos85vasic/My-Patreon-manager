package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestSyncAlias_PrintsDeprecationWarning(t *testing.T) {
	var buf bytes.Buffer
	printSyncDeprecation(&buf)

	got := buf.String()
	for _, want := range []string{
		"deprecated",
		"patreon-manager process",
		"process-command-design.md",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in deprecation warning, got: %q", want, got)
		}
	}
}
