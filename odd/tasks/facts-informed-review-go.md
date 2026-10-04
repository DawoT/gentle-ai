# Feature: facts-informed-review-go

Upstream (fork) side of the Pi dogfooding improvements: make gentle-ai consume
Facts evidence so reviews get cheaper context, evidence-based tiers, and honest
boundary defaults. User approved planning at elite standard; execution TDD,
sprints in order, one reviewable slice per sprint (≤400 lines per PR per
CONTRIBUTING.md:337-358).

Branch: `odd/facts-informed-review-go` (from 5d449e52). Toolchain: go1.27.1.
Baseline verified: `go build ./...` exit 0; `go test ./internal/reviewtransaction
-run TestClassifyRisk` ok.

## Measured motivation (Pi-side evidence)

- Reviewer prompts: 4 lenses × ~116KB near-identical per high review (deltas
  14–46 bytes between lens prompts) — >99% duplicated context.
- Risk signals: crude path heuristics classified tests/docs changes as medium
  (`executable_change` fallback), while real process/auth changes also landed
  medium/high — the signal does not separate them.
- Stale reviewing lineages accumulate; `abandon` exists and works (used live),
  but discovery was poor.

## Contract constraints (never change)

Strict verdict admission (review_provider.go:98–177); exact authority/subject/
revision bindings; immutable frozen-tree evidence; opaque Go-materialized
prompt bytes (the Pi host appends nothing). CLI owns flags/presentation;
reviewtransaction owns evidence, classifier, lifecycle storage.

## Sprints

- [ ] S1 (U1) — Facts digest section in lens prompts. `reviewLensContextBlock`
  (internal/cli/review_lens_context.go:619–724) already has a budgeted
  sectioned composer (`consume`, :658–665). Emit ONE deterministic
  "Facts subject digest" section before patches: declarations/symbol deltas,
  cross-boundary edges, unchangedDependents for changed paths — read from the
  Pi-built facts-commit cache (`.pi/facts-commit-cache`) pinned to the
  candidate tree, read-only, FAIL-OPEN when cache absent (section omitted,
  current bytes otherwise). Digest computed Go-side from the cached database;
  never from the live worktree. Update the scope sentence
  (reviewLensContextInstructionText :737–756) to state the digest's
  evidentiary status. Include the section bytes in START admission and
  materialization budget probes (review_lens_context_test.go).
- [ ] S2 (U2) — Declaration-graph risk signals. At `AssessSnapshotRisk`
  (internal/reviewtransaction/risk.go:264–307): consume the same cached
  database (fail-open absent) to emit canonical reasons — e.g.
  `facts_unchanged_dependents` (N unchanged importers into changed files),
  `facts_docs_only_change`, `facts_symbol_surface_delta` — and make
  ClassifyRisk (:170–187) use them consistently with the existing
  path/mode/passive-proof evidence. Conservative when cache absent: current
  behavior preserved bit-for-bit. Risk reasons must still explain the selected
  tier exactly (:308–313).
- [ ] S3 (U4) — Last consumed boundary as default base. PREMISE GATE FIRST:
  reconcile fork source vs installed v4.0.0 binary behavior (Pi observed a
  fixed-ancestor default; fork source review_facade.go:2134,2229 defaults
  empty → current-changes from HEAD per snapshot.go:1290). If the fork already
  differs, U4 dissolves into a version note. Otherwise: extend the terminal
  consumption tombstone (compact_terminal_consumption.go:19–44) to also store
  the acknowledged candidate tree + branch, consumed during START AND
  STATUS/ASSESS target derivation, with explicit policy for branch switches,
  rewinds, and dirty tracked content; never silently omit work.
- [ ] S4 (U3 dissolved) — Documentation: `review abandon` already covers stale
  reviewing lineages (used live; quarantines with audit proof). Add a docs
  pointer in the facade/README so operators discover it; no new verb.
- [ ] S5 — Closure: `go test ./...`, gofmt, per-sprint commits (Conventional,
  ≤400 lines), PR packaging per CONTRIBUTING.md (approved issue required only
  for upstream PRs; fork-local commits exempt).

## TDD and constraints

Go table-driven tests per package (internal/cli, internal/reviewtransaction);
RED first for each new behavior; fail-open paths tested explicitly; digest
determinism tested (sorted, tree-pinned); budget accounting tested via the
existing probe tests (review_lens_context_test.go:53,209,796,1114). Never
read the live worktree for digest content. AGENTS.md:3–13 skills rule
respected.

## Verification

Per sprint: `go test ./internal/cli -run '...'` and
`go test ./internal/reviewtransaction -run '...'` scoped; `gofmt -l`;
closure: `go test ./...`, `go build ./...`.

## Evidence

Scout handoff (this session): composer seam
internal/cli/review_lens_context.go:619–724,658–665; instruction scope
:737–756; risk pipeline internal/cli/review_assess.go:361,400 →
reviewtransaction/risk.go:264–307,170–187,308–313,751–778,811–827,317–404,
581–675 (8MiB budget :24–26); abandon internal/cli/review_abandon.go:54–91,
:17–35; quarantine compact_abandon.go:238–242; burn compact_burn.go:144–146,
205–211; tombstone compact_terminal_consumption.go:19–49; facade defaults
review_facade.go:2134,2229; snapshot.go:1290; no-ancestor rule
review_operation_contract.go:492–493. No source mutation on this branch yet.
Next: S1 writer (Go, tests-first).
