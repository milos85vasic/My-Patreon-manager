package filter

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"
)

func TestFilterValidPatterns_Direct(t *testing.T) {
	tests := []struct {
		name       string
		patterns   []Pattern
		wantValid  []Pattern
		wantIssues []string
	}{
		{
			name:       "unclosed bracket",
			patterns:   []Pattern{{Raw: "github.com/owner/repo[123"}},
			wantValid:  nil,
			wantIssues: []string{"unclosed bracket: github.com/owner/repo[123"},
		},
		{
			name:       "unmatched closing bracket",
			patterns:   []Pattern{{Raw: "github.com/owner/repo]"}},
			wantValid:  nil,
			wantIssues: []string{"unmatched closing bracket: github.com/owner/repo]"},
		},
		{
			name:       "trailing whitespace negation",
			patterns:   []Pattern{{Raw: "!github.com/owner/neg ", IsNegation: true, Pattern: "github.com/owner/neg "}},
			wantValid:  []Pattern{{Raw: "!github.com/owner/neg", IsNegation: true, Pattern: "github.com/owner/neg", IsRecursive: false}},
			wantIssues: []string{"trailing whitespace: !github.com/owner/neg "},
		},
		{
			name:       "trailing whitespace recursive suffix",
			patterns:   []Pattern{{Raw: "github.com/owner/rec/** ", Pattern: "github.com/owner/rec/** ", IsRecursive: true}},
			wantValid:  []Pattern{{Raw: "github.com/owner/rec/**", Pattern: "github.com/owner/rec/**", IsRecursive: true}},
			wantIssues: []string{"trailing whitespace: github.com/owner/rec/** "},
		},
		{
			name:       "trailing whitespace plain",
			patterns:   []Pattern{{Raw: "github.com/owner/plain ", Pattern: "github.com/owner/plain "}},
			wantValid:  []Pattern{{Raw: "github.com/owner/plain", Pattern: "github.com/owner/plain", IsRecursive: false}},
			wantIssues: []string{"trailing whitespace: github.com/owner/plain "},
		},
		{
			name:       "valid bracket pattern",
			patterns:   []Pattern{{Raw: "github.com/owner/repo[123]", Pattern: "github.com/owner/repo[123]"}},
			wantValid:  []Pattern{{Raw: "github.com/owner/repo[123]", Pattern: "github.com/owner/repo[123]", IsRecursive: false}},
			wantIssues: nil,
		},
		{
			name: "multiple patterns",
			patterns: []Pattern{
				{Raw: "github.com/owner/repo[123", Pattern: "github.com/owner/repo[123"},
				{Raw: "github.com/owner/repo]", Pattern: "github.com/owner/repo]"},
				{Raw: "github.com/owner/ok", Pattern: "github.com/owner/ok"},
			},
			wantValid: []Pattern{{Raw: "github.com/owner/ok", Pattern: "github.com/owner/ok", IsRecursive: false}},
			wantIssues: []string{
				"unclosed bracket: github.com/owner/repo[123",
				"unmatched closing bracket: github.com/owner/repo]",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotValid, gotIssues := filterValidPatterns(tt.patterns)
			// Compare slices ignoring order maybe
			if len(gotValid) != len(tt.wantValid) {
				t.Errorf("filterValidPatterns() valid count = %d, want %d", len(gotValid), len(tt.wantValid))
			} else {
				for i := range gotValid {
					if gotValid[i].Raw != tt.wantValid[i].Raw || gotValid[i].Pattern != tt.wantValid[i].Pattern ||
						gotValid[i].IsNegation != tt.wantValid[i].IsNegation || gotValid[i].IsRecursive != tt.wantValid[i].IsRecursive {
						t.Errorf("pattern %d mismatch: got %+v, want %+v", i, gotValid[i], tt.wantValid[i])
					}
				}
			}
			// issues: compare sets
			if len(gotIssues) != len(tt.wantIssues) {
				t.Errorf("filterValidPatterns() issues count = %d, want %d", len(gotIssues), len(tt.wantIssues))
			} else {
				issueMap := make(map[string]bool)
				for _, iss := range gotIssues {
					issueMap[iss] = true
				}
				for _, want := range tt.wantIssues {
					if !issueMap[want] {
						t.Errorf("missing issue: %s", want)
					}
				}
			}
		})
	}
}

func TestParseRepoignoreFile_PermissionDenied(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".repoignore")
	if err := os.WriteFile(path, []byte("github.com/owner/repo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Remove read permission
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0644)
	_, err := ParseRepoignoreFile(path)
	if err == nil {
		t.Error("expected error for permission denied")
	}
}

func TestParseRepoignoreFile_ScannerTokenTooLong(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".repoignore")
	// Create a line longer than bufio.MaxScanTokenSize (64*1024)
	// Add 10 extra bytes to ensure overflow
	line := make([]byte, bufio.MaxScanTokenSize+10)
	for i := range line {
		line[i] = 'a'
	}
	line = append(line, '\n')
	if err := os.WriteFile(path, line, 0644); err != nil {
		t.Fatal(err)
	}
	_, err := ParseRepoignoreFile(path)
	if err == nil {
		t.Error("expected scanner error for token too long")
	}
}

func TestHasDirective(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		directive string
		want      bool
	}{
		{
			name:      "no-illustration directive present",
			content:   "no-illustration\n",
			directive: "no-illustration",
			want:      true,
		},
		{
			name:      "no-illustration directive absent",
			content:   "github.com/owner/repo\n",
			directive: "no-illustration",
			want:      false,
		},
		{
			name:      "multiple directives",
			content:   "no-illustration\nno-sync\n",
			directive: "no-illustration",
			want:      true,
		},
		{
			name:      "non-matching directive",
			content:   "no-illustration\n",
			directive: "no-sync",
			want:      false,
		},
		{
			name:      "directive with pattern",
			content:   "no-illustration\ngithub.com/owner/*\n",
			directive: "no-illustration",
			want:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, ".repoignore")
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatal(err)
			}
			r, err := ParseRepoignoreFile(path)
			if err != nil {
				t.Fatal(err)
			}
			got := r.HasDirective(tt.directive)
			if got != tt.want {
				t.Errorf("HasDirective(%q) = %v, want %v", tt.directive, got, tt.want)
			}
		})
	}
}
