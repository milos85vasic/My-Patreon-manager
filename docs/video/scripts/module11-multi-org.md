# Module 11: Multi-Organization Repository Scanning

Target length: 12 minutes
Audience: operators, advanced users

## Scene list

### 00:00 — What multi-org support is (60s)
Narration: "When you manage repositories across several GitHub organizations or GitLab groups, scanning them one at a time is tedious. Multi-org support lets the orchestrator discover and deduplicate repositories across all organizations in a single `process` run."

### 01:00 — New environment variables (2m)
[SCENE: IDE showing .env.example]
Narration: "Four new variables control scope. GITHUB_ORGS accepts a comma-separated list of organization logins. GITLAB_GROUPS does the same for GitLab groups. GITFLIC_ORGS and GITVERSE_ORGS cover the remaining providers. Leave any variable empty and the provider falls back to scanning the authenticated user's personal scope — full backward compatibility."
Commands:
    grep -E 'ORGS|GROUPS' .env.example

### 03:00 — Single org vs multiple orgs (2m)
[SCENE: terminal]
Commands:
    # Single org — same as before
    export GITHUB_ORGS=my-company
    ./patreon-manager process --dry-run

    # Multiple orgs
    export GITHUB_ORGS=my-company,open-source-team,partner-org
    ./patreon-manager process --dry-run
Narration: "A single value works identically to the previous behavior. Multiple values are split on commas, trimmed of whitespace, and iterated sequentially with per-request rate limiting."

### 05:00 — How the orchestrator discovers repos (2m)
[SCENE: IDE showing internal/providers/git/github.go]
Narration: "For each organization in the list, the provider calls the list-repositories-for-org endpoint. Results are appended to a shared slice. The same pattern applies to GitLab groups via the group projects API, and to GitFlic and GitVerse via their respective listing endpoints. Pagination is handled per-org — a large organization with hundreds of repos is fully walked before moving to the next."

### 07:00 — Deduplication behavior (2m)
[SCENE: IDE showing internal/services/sync/dedup.go or equivalent]
Narration: "Cross-org deduplication uses the same mirror detection pipeline you learned in Module 5: exact name match, README hash, and commit SHA comparison. If the same repository appears under two organizations — for example a fork used in a partner org — it is counted once and linked to the canonical source. The dedup log entry includes both organization names so you can verify the decision in the audit trail."

### 09:00 — Backward compatibility (90s)
Narration: "If you do not set any of the new variables, behavior is unchanged. The orchestrator scans the token owner's personal repositories, exactly as it did before this feature existed. No migration, no config changes required. You can adopt multi-org incrementally — configure GITHUB_ORGS this week and GITLAB_GROUPS next month."

### 10:30 — Demo walkthrough (90s)
[SCENE: terminal]
Commands:
    export GITHUB_ORGS=org-alpha,org-beta
    export GITLAB_GROUPS=backend-team,infra
    ./patreon-manager process --dry-run --json | jq '.summary'

### 12:00 — Exercise

## Exercise
1. Set `GITHUB_ORGS` to two organizations you have read access to.
2. Run `process --dry-run` and confirm both organizations appear in the report.
3. Create a fork of a repo from one org into the other, re-run `process --dry-run`, and verify the deduplication log shows the mirror entry.

## Resources
- docs/website/content/docs/multi-org-support.md
- docs/guides/git-providers.md
