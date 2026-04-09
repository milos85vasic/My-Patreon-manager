package filter_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/services/filter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRepoignoreFile_NonExistent(t *testing.T) {
	r, err := filter.ParseRepoignoreFile("/nonexistent/.repoignore")
	assert.NoError(t, err)
	assert.NotNil(t, r)
}

func TestParseRepoignoreFile_CommentsAndBlanks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".repoignore")
	err := os.WriteFile(path, []byte("# comment\n\n  \n# another\n"), 0644)
	require.NoError(t, err)

	r, err := filter.ParseRepoignoreFile(path)
	assert.NoError(t, err)
	_ = r
}

func TestParseRepoignoreFile_BasicPatterns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".repoignore")
	err := os.WriteFile(path, []byte("github.com/owner/repo1\ngitlab.com/owner/repo2\n"), 0644)
	require.NoError(t, err)

	r, err := filter.ParseRepoignoreFile(path)
	assert.NoError(t, err)
	assert.True(t, r.Match("https://github.com/owner/repo1"))
	assert.False(t, r.Match("https://github.com/other/repo"))
}

func TestParseRepoignoreFile_Negation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".repoignore")
	err := os.WriteFile(path, []byte("github.com/owner/*\n!github.com/owner/keep\n"), 0644)
	require.NoError(t, err)

	r, err := filter.ParseRepoignoreFile(path)
	assert.NoError(t, err)

	assert.True(t, r.Match("https://github.com/owner/repo1"))
	assert.False(t, r.Match("https://github.com/owner/keep"))
}

func TestRepoignore_Match_ExactMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".repoignore")
	err := os.WriteFile(path, []byte("github.com/owner/repo\n"), 0644)
	require.NoError(t, err)

	r, err := filter.ParseRepoignoreFile(path)
	require.NoError(t, err)

	assert.True(t, r.Match("https://github.com/owner/repo"))
	assert.True(t, r.Match("git@github.com:owner/repo.git"))
	assert.False(t, r.Match("https://github.com/other/repo"))
}

func TestRepoignore_Match_Wildcard(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".repoignore")
	err := os.WriteFile(path, []byte("github.com/owner/*\n"), 0644)
	require.NoError(t, err)

	r, err := filter.ParseRepoignoreFile(path)
	require.NoError(t, err)

	assert.True(t, r.Match("https://github.com/owner/repo1"))
	assert.True(t, r.Match("https://github.com/owner/repo2"))
	assert.False(t, r.Match("https://github.com/other/repo1"))
}

func TestRepoignore_Match_DoubleStar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".repoignore")
	err := os.WriteFile(path, []byte("**\n"), 0644)
	require.NoError(t, err)

	r, err := filter.ParseRepoignoreFile(path)
	require.NoError(t, err)

	assert.True(t, r.Match("https://github.com/any/repo"))
}

func TestRepoignore_Match_CharClass(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".repoignore")
	err := os.WriteFile(path, []byte("github.com/owner/repo[123]\n"), 0644)
	require.NoError(t, err)

	r, err := filter.ParseRepoignoreFile(path)
	require.NoError(t, err)

	assert.True(t, r.Match("https://github.com/owner/repo1"))
	assert.True(t, r.Match("https://github.com/owner/repo2"))
	assert.False(t, r.Match("https://github.com/owner/repo4"))
}

func TestRepoignore_Match_URLNormalization(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".repoignore")
	err := os.WriteFile(path, []byte("github.com/owner/repo\n"), 0644)
	require.NoError(t, err)

	r, err := filter.ParseRepoignoreFile(path)
	require.NoError(t, err)

	assert.True(t, r.Match("https://github.com/owner/repo"))
	assert.True(t, r.Match("http://github.com/owner/repo"))
	assert.True(t, r.Match("git@github.com:owner/repo.git"))
	assert.True(t, r.Match("GITHUB.COM/OWNER/REPO"))
}

func TestRepoignore_NoPatterns(t *testing.T) {
	r := &filter.Repoignore{}
	assert.False(t, r.Match("https://github.com/owner/repo"))
}

func TestRepoignore_Recursive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".repoignore")
	err := os.WriteFile(path, []byte("github.com/owner/**\n"), 0644)
	require.NoError(t, err)

	r, err := filter.ParseRepoignoreFile(path)
	require.NoError(t, err)

	assert.True(t, r.Match("https://github.com/owner/repo"))
	assert.True(t, r.Match("https://github.com/owner/repo/sub"))
}
