# Feature: facts-informed-review-go

## Objective and constraints

Consume Pi Facts evidence in Go-side review prompts and risk signals without
changing verdict admission, authority, immutable tree bindings or prompt ownership.
Repository: user-authorized fork `/home/deuz/projects/gentle-ai`.
Branch: `odd/facts-informed-review-go`, originally from `5d449e52`.
Delivery: local work-unit commits only; no push, PR or merge authorization.
Upstream PRs require an approved issue and contribution-policy checks.

## Historical work and receipts

- [x] S1 — Optional budgeted Facts subject digest, `8d63b250`.
  Review `review-8827dc7ba2d2a78f`, approved and consumed (historical evidence).
  Cache miss must omit the section, never substitute live worktree content.
- [ ] S2 — Declaration-graph risk signals, original `fa80c920`, receipt
  `review-2c6651e012703454`, consumed `ecc33345…`; reopened by live failure.
  Canonical reasons: facts_tests_only_change, facts_unchanged_dependents,
  facts_symbol_surface_delta. Original 10-case matrix and package suite passed.
  Informational backlog: base-walk bounding, classifier divergence, failure coverage.
- [x] S3 — Fixed-ancestor premise disproved; no new boundary authority.
- [x] S4 — Abandon discoverability docs and premise characterization `7c6bae23`.
  Review `review-de227db1f4183221`, approved and consumed (historical evidence).
- [ ] S5 — Closure reopened. Historical run: 79 packages pass, pre-existing
  internal/tui failure, gofmt/vet/build passed. Installed binary was rebuilt by
  user and contains digest/signal strings; strings alone do not prove live behavior.

Original motivation: high reviews repeated ~116KB per lens; test-only executable
changes defaulted to medium; stale lineage retirement lacked discoverability.
Prompt-size comparison (~39KB vs ~116KB) alone does not prove digest injection.
Historical receipts above do not cover the corrective work below.

## Resume reconciliation and incident

Memory previously marked all sprints complete while this file was stale. Preserve
receipt history but reopen invalidated outcomes. `8c6749fb` was committed despite
TestFactsRiskSignals/unresolved failing. Its commit message incorrectly claimed
Pi generations are independent: they have checksummed parent chains. Temporary
DBG guard names were misleading: observed failure was ModuleEdges validation,
not blob mismatch or proof that Markdown paths caused it.

Producer contract: unresolved node/bare imports can legitimately lack a local
target; unresolved relative imports, missing specifiers/importers and unsupported
resolution are not complete graph evidence. Arbitrary continue weakens coverage.
No reset, history rewrite or baseline-test expectation relaxation is authorized.
Existing unrelated edits in gentle-shell remain untouched.

## Corrective tasks

Classification: R1 x M, classifier logic only, no authority/admission mutation.
Route: explorer -> bounded single writer -> independent verifier -> native review
under the user-owned switch. Incident and multi-file delegation triggers apply.
Forecast: 180-300 authored changed lines, one corrective work-unit commit.

- [ ] L1 — IN PROGRESS: restore conservative coverage guards and original
  parent-chain base discovery. Decode specifier/reason; ignore only well-formed
  unresolved builtin/external imports. Preserve checksums and exact tree/blob
  binding. Missing coverage, unresolved relative imports, malformed edges and
  unsupported resolution must preserve baseline risk. TDD: unchanged unresolved
  medium expectation plus realistic external/builtin positives and negative cases.
  Allowed source surfaces: internal/reviewtransaction/facts_risk_signals.go and
  internal/reviewtransaction/facts_risk_signals_test.go. No broad generation search.
- [ ] L2 — Independent package verification, gofmt/vet/build; assess/review the
  corrective slice, commit with accurate checks and evidence. Failures keep tasks
  open. Native review is not functional verification.
- [ ] L3 — Rebuild selected local runtime only after checks pass. Demonstrate
  live assessment on exact matching cache/tree pairs, tests-only positive and
  incomplete-evidence negative control. Verify S1 separately or report pending.

## Verification and acceptance

Commands: go test ./internal/reviewtransaction -run TestFactsRiskSignals -count=1;
go test ./internal/reviewtransaction -count=1; go test ./internal/cli -count=1;
go vet ./internal/reviewtransaction ./internal/cli; go build ./...; scoped gofmt.
Closure: go test ./... with exact pre-existing failures reported separately.

Unresolved/missing evidence never reduces risk. Legitimate builtin/external
imports no longer suppress otherwise complete evidence. Keep baseline classifier
and authority unchanged. Remove temporary diag_live_test.go (already absent at
resume). No installed binary replacement before verification.

Next: L1 writer, test-first; L2 and L3 remain pending.
