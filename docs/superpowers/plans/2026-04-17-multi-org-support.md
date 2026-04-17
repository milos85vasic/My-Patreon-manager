# Plan: Multi-Organization Support for Git Providers

## Overview
Extend My-Patreon-Manager to support scanning repositories across multiple organizations per Git provider. Add environment variables for comma-separated org lists and wildcard support. Update all four Git providers (GitHub, GitLab, GitFlic, GitVerse) to handle multiple orgs and user repository fallback.

## Architecture Changes
- Add new config fields: `GitHubOrgs`, `GitLabGroups`, `GitFlicOrgs`, `GitVerseOrgs`
- Parse comma-separated lists, support wildcard "*" for "all orgs user can access"
- Update `RepositoryProvider` usage: orchestrator will iterate over orgs for each provider
- Ensure backward compatibility: empty org list defaults to user repositories (if provider supports)

## Tasks

### Task 1: Add multi-org environment variables to internal/config/config.go
**File:** `internal/config/config.go`
**Changes:**
1. Add four new string fields to `Config` struct:
   - `GitHubOrgs string`
   - `GitLabGroups string`
   - `GitFlicOrgs string`
   - `GitVerseOrgs string`
2. Update `LoadEnv` function to read environment variables:
   - `GITHUB_ORGS` (default: empty)
   - `GITLAB_GROUPS` (default: empty)
   - `GITFLIC_ORGS` (default: empty)
   - `GITVERSE_ORGS` (default: empty)
3. Add validation (optional): ensure fields are strings, no need for complex parsing yet.
**Test:** Run `go test ./internal/config -v` to ensure existing tests pass.

### Task 2: Update .env.example with new org variables
**File:** `.env.example`
**Changes:** Add four commented lines with examples:
```
# GITHUB_ORGS=my-org,other-org,* (wildcard for all orgs)
# GITLAB_GROUPS=my-group,subgroup/*
# GITFLIC_ORGS=my-org
# GITVERSE_ORGS=my-org
```
**Test:** Verify file syntax.

### Task 3: Create provider config parsing logic in internal/providers/git/provider_config.go
**File:** `internal/providers/git/provider_config.go` (new file)
**Changes:**
1. Create `ParseOrgList(raw string) ([]string, error)` function:
   - Split by comma, trim spaces
   - Handle empty string → returns empty slice
   - Handle "*" wildcard → returns special marker `["*"]`
   - Return slice of org names
2. Create `ShouldScanAllOrgs(orgs []string) bool` helper.
3. Export these functions for use by providers.
**Test:** Write unit tests in `provider_config_test.go` covering:
   - Empty string → empty slice
   - Single org → single element slice
   - Multiple orgs with spaces
   - Wildcard "*" → ["*"]
   - Invalid characters? (just pass through)

### Task 4: Update GitHub provider to handle multiple orgs
**File:** `internal/providers/git/github.go`
**Changes:**
1. Modify `ListRepositories` to accept org string (already does).
2. Update caller side (orchestrator) later; for now ensure GitHub provider can handle empty org (user repos) and org names.
3. No changes needed to actual API calls; GitHub provider already supports both user and org repos.
**Test:** Run existing tests: `go test ./internal/providers/git -run GitHub`

### Task 5: Update GitLab provider to support user repositories and multiple groups
**File:** `internal/providers/git/gitlab.go`
**Changes:**
1. Update `ListRepositories` to handle empty org string (user projects).
   - If org == "" → call `users/{user}/projects` API
   - If org != "" → call `groups/{group}/projects` API (existing)
2. Support multiple groups via orchestrator iteration.
3. Ensure pagination works for both endpoints.
**Test:** Update existing tests and add test for empty org case.

### Task 6: Update GitFlic provider to support user repositories and multiple orgs
**File:** `internal/providers/git/gitflic.go`
**Changes:**
1. Research GitFlic API: find user repos endpoint (likely `/user/repos` or `/users/{username}/repos`).
2. Update `ListRepositories` to handle empty org string.
3. Support multiple orgs via iteration.
**Test:** Update tests accordingly.

### Task 7: Update GitVerse provider to support user repositories and multiple orgs
**File:** `internal/providers/git/gitverse.go`
**Changes:** Similar to GitFlic.
**Test:** Update tests.

### Task 8: Update orchestrator to scan multiple orgs
**File:** `internal/services/sync/orchestrator.go`
**Changes:**
1. Add method `orgsForProvider(providerName string) []string` that reads config and parses org list.
2. In `scanRepositories` (or similar), iterate over orgs for each provider.
3. For each org, call `provider.ListRepositories`.
4. Deduplicate repositories across orgs (by full name).
5. Handle wildcard "*" – need to fetch all orgs user belongs to (stretch goal: implement later, for now treat as empty list).
**Test:** Update orchestrator tests to mock multi-org scanning.

### Task 9: Update CLI to accept org lists
**File:** `cmd/cli/main.go`
**Changes:** Optional. Add `--org` flag that overrides env var. Keep backward compatibility.
**Test:** Ensure existing CLI tests pass.

### Task 10: Create documentation
**File:** `docs/multi-org-support.md`
**Content:** Explain new environment variables, examples, wildcard support, provider-specific notes.
**Test:** None.

## Success Criteria
- All existing tests pass (100% coverage)
- Can scan multiple GitHub organizations in one sync
- GitLab, GitFlic, GitVerse support user repositories (empty org)
- Backward compatible: empty org list scans user repos
- Documentation updated