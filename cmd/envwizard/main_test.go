package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestEnvWizardHelp(t *testing.T) {
	cmd := exec.Command("go", "run", "./cmd/envwizard", "--help")
	cmd.Dir = repoRoot(t)
	output, _ := cmd.CombinedOutput()
	assertContains(t, string(output), "env")
}

func TestEnvWizardBuild(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "envwizard-build-*")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	cmd := exec.Command("go", "build", "-o", tmpFile.Name(), "./cmd/envwizard")
	cmd.Dir = repoRoot(t)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %s: %v", output, err)
	}

	info, err := os.Stat(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("built binary is empty")
	}
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !contains(s, substr) {
		t.Fatalf("expected %q to contain %q", s, substr)
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
