# Feature: facts-informed-review-go

## Scope and authorization

Consume Pi Facts in Go-side prompts/risk without changing review authority,
verdict admission, frozen tree/blob bindings or prompt ownership.
Fork /home/deuz/projects/gentle-ai; branch odd/facts-informed-review-go.
User explicitly prioritizes live dogfooding and authorizes rebuilding the selected
dev runtime despite native reviewer quota. Local commits authorized; no push,
PR, merge, model switch, RDD-disable or implicit approval. Preserve unrelated edits.

## Historical units

- [x] S1: optional digest 8d63b250; review-8827dc7ba2d2a78f consumed.
- [ ] S2: signals fa80c920; review-2c6651e012703454 consumed ecc33345…;
  reopened for real-cache compatibility. Facts reasons: tests-only, dependents,
  symbol surface delta. Informational backlog: base-walk bound/tier/failure coverage.
- [x] S3/S4: disproved fixed-ancestor premise, abandon discoverability 7c6bae23;
  review-de227db1f4183221 consumed.
- [ ] S5: closure reopened; historical 79 packages pass, pre-existing internal/tui
  failure. Full suite has not been rerun. Strings/prompt size do not prove S1 live.

## Corrective units and evidence

- [x] L1: a3bab16db4ed5a2d3712a748ad5205dd4fc9bbc0. Conservative guards and
  parent-chain base discovery restored; narrow builtin/external exemptions;
  accepts resolved filesystem/typescript. Unresolved RED (low vs medium) -> GREEN
  unchanged assertion; TypeScript RED (medium vs high) -> GREEN. Independent
  focused matrix and reviewtransaction/cli suites, vet/build/gofmt/diff-check pass.
  CLI initial120s timeout had no verdict, foreground rerun passed. Mechanical
  isolation not certified. 333 authored commit lines, 316 native frozen lines.
- [ ] L2: BLOCKED review-786a02b33e841875 reviewing/collect, medium/reliability.
  HTTP429 code1310 reviewer quota reset 2026-10-08 23:38:21 (timezone unspecified).
  Zero prepared/submitted verdicts, no approval/ack. Fresh STATUS reconciled.
  Invalid bindingRef/correctionLines attempts were parent orchestration errors,
  no authority mutation. Resume exact fresh STATUS; do not replay old binding.
- [ ] L3: PENDING remaining positive live acceptance; installed rebuild verified.
  Rebuilt a3bab16d dev binary SHA576e0970b0480e425aeb2ca84c4ad7251532ffcad10a7303e7c369f5986b1171;
  old/backup SHA c00872bc3bed7228c2d5ecc1c7ccb9578b341c1c5ce143a0c3f87a779bdfb798.
  Selected .gentle-ai/v4.0.0/gentle-ai via existing dev registration; registration
  and pinned integrity metadata unchanged. Revision exact, vcs.modified=true,
  tracking was dirty; no clean-build claim. Backup under .gentle-ai/dogfooding/a3bab16d.
- [ ] L4: IN PROGRESS. Live-discovered filesystem JSON resources outside Facts
  source inventory must be verified against exact frozen trees, not blindly
  skipped. Batch/bound lookup; accept only verified JSON resource vertices.
  Missing/invalid/unresolved/unsupported targets remain misses. Test import of
  production resource must not lower tier; changed unindexed JSON stays conservative.
  TDD scoped to facts_risk_signals.go and facts_risk_signals_test.go, then independent
  checks, corrective commit, rebuild and identical live comparison.

Incident retained: unsafe 8c6749fb was committed despite unresolved failure.
Its independent-generation/Markdown root-cause claims were false/unproven;
actual guard was ModuleEdges, not blobs. No history rewriting or test relaxation.

## Real live probe (all immutable real data)

Registered same-clone worktrees under parent gentle-shell .pi/:
facts-live-07cefa0b (real cache), facts-live-control-07cefa0b (no cache).
Candidate07cefa0b5669a70d323fc3ae3005832fe54bfd0e;
base d11971cf5d5c8b2080a362a4d3b6b71b092b71db.
Trees candidate1aa6f9c1ff4e7a7dd9799fb77c2588df4f423da8,
base861563e993400a03d9e219dac3f35cea93b143c5.
Four paths all tests: atomic-marker.test.ts, facts-commit.test.ts,
support/atomic-marker.mjs, support/facts-probe-worker.mjs (all under tests/).
Real indexFactsCommit, base then candidate: 563/565 files,3116/3127edges,omitted[].
Generations66ceaac8863e4b96ec6504b918655a2a3a4e8fd89920ec1fe6b40b647ff737f0
and997259cd97c463b90727cb55e59a48e1c4897b575666506db171b9257a11ed30,
depths0/1 correct parent. Main cache never changed.
Initial assess exit1: expected-untracked-inventory missing, before Facts.
Facade inspect supplied sha256:0fc9c89f9fb57a5340cb06a0d61707dbf4d16b8d18d78a55fd8c28d114b47735.
Both valid assessments exit0 medium/executable_change tests/atomic-marker.test.ts,
4paths/191lines, no Facts reason. Independent reconstruction: checksums/blobs all
match;24edges per snapshot lack indexed target. First runtime-metrics-native.ts
-> contracts/telemetry/runtime-aggregate-v1.schema.json (filesystem). Target exists
in Git. Source-mapped conservative miss, not runtime branch instrumentation.
Positive S2 and S1 live injection remain unverified; GO-S1 source untouched.

## Checks and routing

R1xM: explorer -> one bounded writer -> independent verifier. Runtime rebuild has
no meaningful TDD RED; source behavior correction does. Commands: focused
TestFactsRiskSignals; go test ./internal/reviewtransaction ./internal/cli -count=1;
go vet those packages; go build ./...; scoped gofmt; diff-check. Closure full suite
pending. L4 forecast ~180-300 authored diff lines including docs, one work unit.
Next: L4 JSON-resource fix, then L3 rebuild/live; L2 remains blocked independently.
