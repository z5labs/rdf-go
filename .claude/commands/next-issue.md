---
description: Take the next eligible story issue through the full dev cycle — worktree, implement, test, PR, checks, Copilot review, merge.
allowed-tools: Bash, Read, Write, Edit, Glob, Grep, EnterWorktree, ExitWorktree, Monitor, TaskCreate, TaskUpdate
---

Run **exactly one** issue end-to-end, then stop. Do not start a second issue in the same
invocation — the loop re-invokes this command for the next one.

Repo: `z5labs/rdf-go`. Default branch: `main`. Milestone under construction: `v0.1.0`.

## 1. Pick the issue

```
gh issue list --state open --label story --milestone v0.1.0 --limit 100 --json number,title --jq 'sort_by(.number)[]'
```

Always pass `--limit` — the default page size is 30 and this backlog is larger, so
without it eligible issues silently vanish from the list.

Walk the list in ascending number order. For each candidate, read its body
(`gh issue view <n>`) and look at the **Related Issues** section for `Blocked by:` lines
listing `- #N`. An issue is *eligible* only when every issue it lists is CLOSED
(`gh issue view <N> --json state`). Take the lowest-numbered eligible issue.

Dependencies live in the issue body text only. GitHub's native `blocked-by` field is
empty on this repo — do not trust it.

- If no open story issues remain in the milestone, print `BACKLOG EMPTY` and stop.
- If open issues remain but none are eligible, print `BLOCKED` plus which dependency is
  holding things up, and stop.

Read the whole issue body. The **Acceptance Criteria** checklist is the spec — every box
must be genuinely satisfied before you open the PR.

## 2. Worktree

```
EnterWorktree(name: "issue-<n>")
```

This branches fresh from `origin/main`, so previously merged work is present. Confirm with
`git rev-parse --show-toplevel` and `git log --oneline -3`.

Never commit, branch, or open a PR from the main checkout at
`/home/carson/github.com/z5labs/rdf-go`. Avoid bare `git stash` — the stash stack is
shared across worktrees.

If the repo root has a `CLAUDE.md`, read it now — it holds the implementation conventions
later stories must follow.

## 3. Implement

Work through the acceptance criteria in order. Follow the conventions already established
in the package rather than inventing new ones.

The package layout is **versioned by RDF spec** — confirmed from the issue bodies, do not
flatten it:

- `rdf` (root) — term types shared across both spec versions
- `rdf11/ntriples`, `rdf11/nquads`, `rdf11/turtle`, `rdf11/trig`
- `rdf12/ntriples`, `rdf12/nquads`, `rdf12/turtle`, `rdf12/trig`
- `internal/lex` — shared lexical primitives (#22)
- `iri`, `vocab`

The point of the split is that a graph produced by an `rdf11/*` package can be handed to
an `rdf12/*` printer and back, so the shared term types in `rdf` model RDF 1.2 features
(base direction, `rdf:dirLangString`) even during the 1.1 milestone.

Other conventions:

- Zero third-party runtime dependencies. Do not add a `tool` directive to `go.mod` — it
  leaks into the dependency graph of everyone importing the library.
- Table-driven tests with named subtests, in the style the issues call for.
- Exported identifiers get doc comments starting with the identifier name.
- Cite the grammar production or RFC section that a piece of parsing logic implements.

Write tests alongside the implementation.

## 4. Verify locally

All four must pass before you go further — this is the same set CI runs, so a local
failure is a guaranteed red PR:

```
go build ./...
go vet ./...
go test -race ./...
staticcheck ./...
```

Also run `gofmt -l .` and fix anything it lists.

`staticcheck` must be 2026.1 or newer (`staticcheck --version`). An older build fails with
`module requires at least go1.25, but Staticcheck was built with go1.23` — that is a stale
local binary, not a code defect. Reinstall with
`go install honnef.co/go/tools/cmd/staticcheck@2026.1`.

If a test fails, fix the code — never weaken the test to make it pass. If the acceptance
criteria and a passing test genuinely conflict, stop and report rather than guessing.

## 5. Commit and open the PR

```
git add -A
git commit -m "<type>(<scope>): <summary>"   # match the issue title's prefix
git push -u origin HEAD
gh pr create --title "<issue title>" --body "<body>"
```

The PR body must include `Closes #<n>` so merging closes the issue, the acceptance
criteria checked off one by one, a note on any judgment call that shaped the public API,
and how it was verified. Keep the standard Claude Code attribution. End commit messages
with `Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>`.

## 6. Wait for checks

Do not foreground a `sleep` loop. Either use `gh pr checks <pr> --watch --fail-fast`, or
run the poll with Bash `run_in_background` so a single notification arrives on completion.

- Exit 0 → green, continue.
- "no checks reported" → CI is missing; investigate rather than treating it as a pass.
  CI landed in #38 and runs on every PR.
- Failure → read the logs (`gh run view <id> --log-failed`), fix, push, and re-watch.
  After **three** failed attempts on the same root cause, stop and report instead of
  looping.

## 7. Request Copilot review

Copilot is a **Bot**, not a User. Two things matter: the login is
`copilot-pull-request-reviewer[bot]`, and passing a bare `reviewers[]=Copilot` to REST
returns 200 while silently doing nothing.

The GraphQL mutation is the reliable path:

```
PR_ID=$(gh pr view <pr> --json id --jq .id)
BOT_ID=$(gh api '/users/copilot-pull-request-reviewer[bot]' --jq .node_id)
gh api graphql -f query='
mutation($pr:ID!, $bot:ID!) {
  requestReviews(input: {pullRequestId: $pr, botIds: [$bot], union: true}) {
    pullRequest { reviewRequests(first:10) { nodes {
      requestedReviewer { __typename ... on Bot { login } } } } }
  }
}' -f pr="$PR_ID" -f bot="$BOT_ID"
```

REST also works on this repo *provided the full bot login is used* — verified on #38 and
#39:

```
gh api --method POST repos/z5labs/rdf-go/pulls/<pr>/requested_reviewers \
  -f "reviewers[]=copilot-pull-request-reviewer[bot]" -q '.number'
```

Either way, confirm the request took by checking that the login appears in
`requested_reviewers`; an empty list means it did not.

Then wait for the review to land. Run this with Bash `run_in_background`:

```
for i in $(seq 1 40); do
  n=$(gh api repos/z5labs/rdf-go/pulls/<pr>/reviews --jq 'length' 2>/dev/null || echo 0)
  if [ "$n" -gt 0 ]; then echo "copilot review landed"; exit 0; fi
  sleep 15
done
echo "copilot review timed out"; exit 1
```

If the request itself errors (Copilot review not enabled for the org), note it in your
report and continue without it — do not stall the cycle.

## 8. Address review comments

Pull both the summary review and the inline comments — a review with `"generated no
comments"` in its body still counts as having reviewed:

```
gh api repos/z5labs/rdf-go/pulls/<pr>/reviews --jq '.[] | "\(.user.login) [\(.state)]\n\(.body)"'
gh api repos/z5labs/rdf-go/pulls/<pr>/comments --jq '.[] | "[\(.id)] \(.path):\(.line)\n\(.body)"'
```

Use judgment. Fix what is a real defect or a genuine improvement. Where a comment is wrong
or does not apply, reply on the thread explaining why rather than making the change — do
not silently ignore it and do not change correct code just to clear a comment.

```
gh api --method POST repos/z5labs/rdf-go/pulls/<pr>/comments/<comment-id>/replies \
  -f body="<reply>" -q '.id'
```

If you push fixes, go back to step 6 and let checks re-run before merging.

## 9. Merge

Only once checks are green and every Copilot comment is either addressed or answered:

```
gh pr merge <pr> --squash
```

The repo allows squash merges only and has `deleteBranchOnMerge` enabled, so the remote
branch is removed for you. Do **not** pass `--delete-branch`: from inside a worktree it
makes `gh` try to check out `main` locally and fail with
`'main' is already used by worktree`. The merge still succeeds, but the error reads like
a failure.

Verify the merge and that the issue closed:

```
gh pr view <pr> --json state,mergedAt -q '"\(.state) \(.mergedAt)"'
gh issue view <n> --json state -q .state
```

## 10. Clean up

The merged commit is already on `main`, so the worktree branch is redundant:

```
ExitWorktree(action: "remove", discard_changes: true)
```

Then `git -C /home/carson/github.com/z5labs/rdf-go checkout main && git pull` so the next
iteration branches from the merged state.

## Report

Finish with a short status line: issue number and title, PR number and URL, check result,
whether Copilot reviewed and what it flagged, and merge confirmation. If you stopped early,
say exactly where and why.

## Stop conditions

Stop and report — do not push through — if any of these happen:

- The same CI failure survives three fix attempts.
- Acceptance criteria are ambiguous enough that two readings produce materially different
  public APIs.
- Merging would require a force-push, a branch-protection override, or discarding someone
  else's commits.
- `git status` in the main checkout is dirty with changes you did not make.
