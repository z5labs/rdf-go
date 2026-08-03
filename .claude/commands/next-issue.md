---
description: Take the next eligible story issue through the full dev cycle — worktree, implement, test, PR, checks, Copilot review — then label it for GitHub to auto merge.
allowed-tools: Bash, Read, Write, Edit, Glob, Grep, EnterWorktree, ExitWorktree, Monitor, TaskCreate, TaskUpdate
---

Take **exactly one** issue from backlog to a merged PR, then stop. Do not start a second
issue in the same invocation — the loop re-invokes this command for the next one.

You never run `gh pr merge`. You label the PR and GitHub merges it, gated by main's branch
protection; see step 9.

Repo: `z5labs/rdf-go`. Default branch: `main`. Milestone under construction: `v0.2.0`.

## 1. Pick the issue

```
gh issue list --state open --label story --milestone v0.2.0 --limit 100 --json number,title --jq 'sort_by(.number)[]'
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

`EnterWorktree` and `ExitWorktree` are **unavailable to a subagent running with a
working-directory override** — the tools refuse rather than falling back. When that
happens, use git directly and drive the worktree by absolute path:

```
git fetch origin
git worktree add -b issue-<n> .claude/worktrees/issue-<n> origin/main
```

Branch from `origin/main`, not from local `main`: the main checkout is often behind, since
earlier iterations merge through GitHub rather than locally.

Never commit, branch, or open a PR from the main checkout — the directory holding
`.claude/worktrees/`, which `git worktree list` reports first. Avoid bare `git stash`;
the stash stack is shared across worktrees.

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
poll with `Monitor`.

Do **not** use Bash `run_in_background` for a wait loop. It has been observed exiting
immediately without ever polling, which looks like a completed wait and reports whatever
the first sample happened to be. `Monitor` is the tool for any wait in this cycle.

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

Then wait for the review to land. Poll with `Monitor`, not Bash `run_in_background` — see
the warning in step 6:

```
for i in $(seq 1 40); do
  n=$(gh api repos/z5labs/rdf-go/pulls/<pr>/reviews --jq 'length' 2>/dev/null || echo 0)
  if [ "$n" -gt 0 ]; then echo "copilot review landed"; exit 0; fi
  sleep 15
done
echo "copilot review timed out"; exit 1
```

A non-empty `reviews` array does **not** mean the PR was reviewed. Copilot posts a review
whose body declines the work — most often `"Copilot wasn't able to review this pull request
because it exceeds the maximum number of files (300)"` — and that decline satisfies the
`length > 0` test above. Check the body before treating the review as real:

Read the **most recent** Copilot review only. Reruns and pushed fixes leave older reviews
in the array, so an earlier decline sitting beside a later completed review — or the
reverse — is easy to misread:

```
gh api repos/z5labs/rdf-go/pulls/<pr>/reviews \
  --jq '[.[] | select(.user.login | test("copilot";"i"))]
        | sort_by(.submitted_at) | last | .body // "no copilot review"'
```

A body matching `wasn't able to review` is a **declined** review, not a completed one.
This happened on #57, where the vendored W3C suites pushed the PR past the file limit and
the cycle merged with no review at all.

If the review is declined, times out, or the request itself errors (Copilot review not
enabled for the org), the cycle does **not** merge — see step 9.

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

The `pulls/<pr>/` segment belongs there. Copilot has claimed on this very file that the
route is `pulls/comments/<comment-id>/replies` and that the form above 404s; it is wrong,
and the reply rebutting it was posted with the command as written. The shorter path is the
`GET`/`PATCH`/`DELETE` route for a single review comment, not the reply route.

If you push fixes, go back to step 6 and let checks re-run before labelling the PR.

## 9. Label the PR for auto merge — never merge it yourself

**Do not run `gh pr merge`.** Not with `--auto`, not without. An agent merging to `main` is
what this whole arrangement exists to avoid: the permission classifier stops it, and a
subagent that merges emits a security banner into its caller's context that blocks the
caller's next agent spawn, killing the loop on its second iteration.

Instead, hand the merge to GitHub by labelling the PR:

```
gh pr edit <pr> --add-label auto-merge
```

`.github/workflows/auto-merge.yaml` picks up the `labeled` event and enables native auto
merge. GitHub then squash-merges the PR once every required status check passes — and
leaves it open if one fails. The label cannot skip a check; it only says the human-judgment
conditions below are satisfied.

Apply the label only when **both** hold:

1. Checks are green, and
2. A Copilot review actually **completed** — it either left comments (every one now
   addressed or answered) or reported that it generated none.

The label means *"I verified these two things"*. It does not mean *"skip them"*. Required
status checks are enforced by main's branch protection whatever you do; condition 2 is
enforced by nothing but you.

A review that was declined, never arrived, or was never requested because the request
errored is **not** a completed review. In that case do **not** label the PR. Leave it open,
leave the worktree in place, and stop with a report beginning `BLOCKED` that names the PR
and why the review is missing, so the user can tell a PR that needs a human look from one
that merged on its own.

If the PR is unreviewable because it exceeds the 300-file limit, say so in the `BLOCKED`
report and suggest how the work could be split.

## 10. Wait for the merge, then clean up

Labelling only **queues** the merge. The PR sits in `OPEN` with auto-merge armed until the
required checks report, so cleaning up immediately would remove the worktree out from under
a merge that has not happened yet.

Wait for the PR to actually reach `MERGED` before touching anything. Use `Monitor` for the
wait — Bash `run_in_background` has been observed exiting immediately without ever polling,
which reads as "merged" when nothing was checked:

```
gh pr view <pr> --json state,mergedAt -q '"\(.state) \(.mergedAt)"'
```

If it stays `OPEN` after the checks have finished, something is wrong with the auto-merge
setup — report it rather than merging by hand. If a required check fails, auto merge stays
armed and the PR stays open; fix the failure, push, and it merges when the rerun is green.

Once the state really is `MERGED`, confirm the issue closed and drop the worktree:

```
gh issue view <n> --json state -q .state
git worktree remove .claude/worktrees/issue-<n>
```

Then bring the main checkout up to date so the next iteration branches from the merged
state. `git worktree list` reports it first, so it needs no hard-coded path:

```
git -C "$(git worktree list --porcelain | head -1 | cut -d' ' -f2-)" checkout main
git -C "$(git worktree list --porcelain | head -1 | cut -d' ' -f2-)" pull
```

The repo allows squash merges only and has `deleteBranchOnMerge` enabled, so the remote
branch is removed for you.

## Report

Finish with a short status: issue number and title, PR number and URL, check result,
whether Copilot reviewed and what it flagged, whether the PR reached `MERGED`, and any
judgment call that shaped the public API. If you stopped early, say exactly where and why —
beginning the report with `BLOCKED`.

## Stop conditions

Stop and report — do not push through — if any of these happen:

- The same CI failure survives three fix attempts.
- Acceptance criteria are ambiguous enough that two readings produce materially different
  public APIs.
- Landing the PR would require a force-push, a branch-protection override, or discarding
  someone else's commits.
- `git status` in the main checkout is dirty with changes you did not make.
- The Copilot review declined, timed out, or was never requested successfully (step 9).

When you stop for one of these, begin the report with `BLOCKED` so the caller can tell a
halted cycle from a finished one at a glance.
