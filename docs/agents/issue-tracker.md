# Issue tracker: GitHub

Issues and specs for this repo live in GitHub Issues. Use the `gh` CLI from this clone; it infers the repository from the Git remote. Outside the clone, pass `--repo mertdotcc/kubectl-cron`.

## Conventions

- **Create**: `gh issue create --title "..." --body-file <file>`. Write multiline bodies to a temporary file and pass it with `--body-file`.
- **Read**: `gh issue view <number> --json number,title,body,labels,comments`.
- **List**: `gh issue list --state open --json number,title,body,labels,comments`, with appropriate `--label`, `--state`, and `--limit` filters.
- **Make a sub-issue**: `gh issue create --parent <parent> --title "..." --body-file <file>`, or `gh issue edit <parent> --add-sub-issue <child>`. If sub-issues are unavailable, add the child to a task list in the parent body and put `Part of #<parent>` at the top of the child body.
- **Comment**: `gh issue comment <number> --body-file <file>`.
- **Apply or remove labels**: `gh issue edit <number> --add-label "..."` or `--remove-label "..."`.
- **Close**: `gh issue close <number>`.

## Pull requests as a triage surface

**PRs as a request surface: no.**

When enabled, use the corresponding `gh pr` commands to read, comment, label, and close PRs. Read the diff with `gh pr diff <number>`.

GitHub shares a number space across issues and PRs. Resolve an ambiguous reference with `gh pr view <number>`, falling back to `gh issue view <number>`.

## Skill vocabulary

- **Publish to the issue tracker**: create a GitHub issue.
- **Fetch the relevant ticket**: read the GitHub issue.

## Wayfinding operations

Used by `/wayfinder`. The map is a single issue with child issues as tickets.

- **Map**: an issue labelled `wayfinder:map`, with Notes, Decisions-so-far, and Fog in its body.
- **Child ticket**: a sub-issue of the map, using the fallback in Conventions when necessary. Label it `wayfinder:<type>`, where type is `research`, `prototype`, `grilling`, or `task`.
- **Blocking**: use native issue dependencies: `gh issue edit <child> --add-blocked-by <blocker>`. If unavailable, put `Blocked by: #<number>, #<number>` at the top of the child body. A ticket is unblocked when all blockers are closed.
- **Frontier query**: list the map's open children; exclude assigned tickets and tickets with open blockers. For native dependencies, inspect `issue_dependencies_summary.blocked_by` with `gh api repos/{owner}/{repo}/issues/<child>`. For the fallback, check the referenced blockers. Choose the first eligible child in map order.
- **Claim**: assign the ticket to the driving developer with `gh issue edit <number> --add-assignee @me` when the session is acting for that developer.
- **Resolve**: comment with the answer, close the ticket, then append a context pointer (gist and link) to the map's Decisions-so-far.
