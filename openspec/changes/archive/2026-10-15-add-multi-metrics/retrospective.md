# Retrospective: add-multi-metrics

> Written: 2026-10-15 (after verify passed with warnings)
> Commit range: `3cf7be5..cb7ed09`
> Worktree: main checkout, branch `feature/add-multi-metric`

---

## 0. Evidence

- **Commit range**: `3cf7be5..cb7ed09` (2 commits: `a5f4836` planning
  artifacts, `cb7ed09` implementation)
- **Diff size**: +1876 / -104 across 26 files (implementation slice:
  +1365 / -121 across 17 files)
- **Tasks done**: 17/17 (`grep -cE '^\s*- \[x\]' tasks.md` = 17)
- **Active hours**: ~1 (single session: plan → TDD implementation →
  verify → retro)
- **Subagent dispatches**: 0 (native execution)
- **New external dependencies**: none
- **Bugs encountered post-merge**: none (not yet merged; all tests
  green at `cb7ed09`)
- **OpenSpec validate state at archive**: pass (all items valid,
  INFO-only notes)
- **Test coverage signal**: 9 packages `ok` under `go test ./...`;
  new/extended tests: 2 config tables (multi validation, sub-series
  collisions), `buildRowMap` table, `TestCollectOnceMulti` +
  `TestSchedulerMultiOneQueryPerCycle`, `TestLoadGraphsMetricList` +
  extended rejection table, 3 multi-series chart tests, 3 multi-series
  server tests

Commit chain:

```
3cf7be5 baseline (origin/main)
a5f4836 planning artifacts (brainstorm, proposal, design, delta specs, tasks)
cb7ed09 implementation + plan.md + docs/examples
```

---

## 1. Wins

- [evidence: internal/collect/collector.go, TestCollectOnceMulti] The
  core design decision held exactly: explosion inside `CollectOnce`
  meant zero changes to store, API, and scheduler; the store's
  counter/gauge-only type byte rejected the "multi" type string during
  TDD, confirming sub-series must carry column types.
- [evidence: tasks.md 17/17, make lint/test/build green] Every task
  implemented as specced with no scope creep; the only scheduler work
  (task 4.1) was a confirmation plus a test, as predicted.
- [evidence: internal/web/chart/chart.go palette] Single-series charts
  render byte-identical to before (no inline styles); multi-series opt
  into palette colors — the "index 0 = CSS accent" trick kept
  visual continuity for free.
- [evidence: TestLoadGraphsMetricList "mixed series freely"] The
  name-driven dumb-frontend stance paid off: the frontend diff was
  exactly the two predicted pieces (scalar-or-list metric, N-series
  chart) plus concurrent per-series fetch.

## 2. Misses

- 🟡 [painful | evidence: plan.md Task 5] The chart refactor changed
  `Render`/`Clip` signatures, forcing mechanical edits to every
  existing chart test and server call site in one task. Unavoidable
  for the `[]Series` API but the largest single diff chunk.
- 📌 [nit | evidence: collect_test.go edit, 3 fix-up rounds] Test
  editing had self-inflicted churn: dropped function signature in an
  edit, leftover unused struct fields, missing `strings` import.
  Caught by compiler, zero product impact.
- 📌 [nit | evidence: verify.md §5] Commits not pushed; branch is 2
  ahead of origin/main. User action pending.

## 3. Plan deviations

| Plan task | What changed | Why |
|-----------|--------------|-----|
| 2.2 | Tested `buildRowMap` (pure) instead of live-DB `QueryRow` row counts | Consistent with how `QueryValue` is tested; row-count enforcement stays a thin untested wrapper, as planned |
| 5.2 | Added "empty scalar metric" and "empty entry in list" rejection cases beyond the task list | Review Focus item `metric: ""` / `[""]` — the spec's non-empty requirement pinned it |

## 4. Skill / workflow compliance

| Skill                                            | Used |
|--------------------------------------------------|------|
| superpowers:brainstorming                        | ✓ (via openspec explore; captured in brainstorm.md) |
| superpowers:writing-plans                        | ✓ (plan.md, TDD micro-steps) |
| superpowers:using-git-worktrees                  | ✗ |
| superpowers:subagent-driven-development          | ✗ (native execution, per user's apply invocation) |
| (transitive) superpowers:test-driven-development | ✓ (failing test first for every unit) |
| (transitive) superpowers:requesting-code-review  | ✗ |
| superpowers:finishing-a-development-branch       | ✗ (branch not merged/pushed yet) |

### Deliberately Skipped Skills

- **superpowers:using-git-worktrees**
  - **What was skipped**: creating an isolated worktree for the change
  - **Why this cycle**: the user invoked the native openspec apply flow
    in the main checkout; the change was scaffolded and implemented in
    place on the existing `feature/add-multi-metric` branch, so a
    worktree would have duplicated an already-isolated branch.
  - **How to prevent recurrence**: `scope-judgment rule` — worktree
    only when work would otherwise share a checkout with other
    in-flight work; a dedicated feature branch with a clean worktree
    is equivalent isolation for this repo's single-session changes.
- **superpowers:subagent-driven-development**
  - **What was skipped**: dispatching fresh implementer/reviewer
    subagents per task
  - **Why this cycle**: the user's apply command ran the native path;
    the plan carried full interface decisions, so single-context
    execution was the cheap option.
  - **How to prevent recurrence**: `scope-judgment rule` — offer the
    subagent path when a change spans >2 sessions or needs independent
    review before merge; not needed for a one-session, 17-task,
    fully-tested change.
- **superpowers:requesting-code-review / finishing-a-development-branch**
  - **What was skipped**: fresh-model branch review; merge/PR/push
    flow
  - **Why this cycle**: user asked for the OpenSpec steps only
    (verify → retrospective → archive); branch finishing is a git
    action deliberately left to the user.
  - **How to prevent recurrence**: `one-off — schema boundary case` —
    the OpenSpec archive step and the git branch-finishing step are
    separate workflows by design; the boundary is user intent.

## 5. Surprises

- The store was already the strictest guardrail: writing a
  type-"multi" point failed in `BadgerStore.Write` before the collector
  multi path existed — a free confirmation that the design's
  "sub-series are ordinary counter/gauge points" invariant is enforced
  at the storage layer, not just by convention.

## 6. Promote candidates → long-term learning

- [ ] 📌 **Chart signature changes ripple: prefer additive rendering
  APIs** → **Promote to project CLAUDE.md** (development section)
  > **Why**: changing `Render(points)` to `Render([]Series)` forced
  > rewriting all chart tests and the server call site in one task;
  > additive variants would have kept the diff smaller.
  > **How to apply**: when extending chart (or similar pure-render)
  > packages, consider a new function/type over reshaping existing
  > signatures if call sites and tests are numerous.
- [ ] 🟡 **Dotted sub-series names are stringly-typed** → **One-off**
  > **Why**: `parent.column` collision checking covers config-declared
  > names; query-returned columns with dots can still alias
  > ambiguously at runtime — accepted limitation per design D4, rare in
  > practice.
  > **How to apply**: revisit only if a real PostgreSQL column name
  > with dots collides; not worth generalizing now.
