## Private inputs

Keep private local planning material outside Git history. Never stage, commit, or reproduce its source text in repository files. Write public documentation from confirmed decisions in fresh wording.

## Commits

Use Conventional Commits with short, descriptive subjects. Preserve the configured human author and committer identity. Omit assistant attribution from commit messages and PRs, including co-author trailers.

## Distribution

Never publish releases, create release tags, submit or update Krew entries, or invoke release/submission automation. Keep development, testing, and commits local unless the user explicitly requests a code push.

## Feature work

Implement features sequentially. Finish each feature's verification and focused Conventional Commit before starting the next.

## Agent skills

### Issue tracker

Track issues and specs in GitHub Issues for `mertdotcc/kubectl-cron`. Before issue operations, read `docs/agents/issue-tracker.md`.

### Triage labels

Use the five default triage labels. Before triaging issues, read `docs/agents/triage-labels.md`.

### Domain docs

Single-context layout: root `GLOSSARY.md` and `docs/adr/`. Before exploring the codebase, read `docs/agents/domain.md`.
