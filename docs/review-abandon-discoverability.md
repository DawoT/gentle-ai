# Retire stale review lineages with `review abandon`

Use `gentle-ai review abandon` to quarantine a stale, non-terminal compact-v2
review lineage you have deliberately decided not to finish. It works from
persisted authority even when the live worktree has drifted. No new cleanup verb
or manual deletion of review storage is needed.

## Inspect, bind, abandon

1. Run `gentle-ai review status --cwd <repo>` **without a contract** to inspect
   the persisted authority inventory. Find the lineage's `entries[]` row and
   read `lineage_id`, `revision`, `snapshot_identity`, and `discarded_work`.
2. Choose an actor and a reason: `operator_disposition` for deliberate retirement,
   or `retired_schema` for schema retirement. Review the work you will discard.
3. Fill the following binding from that row. Join the eight lines with LF only,
   with **no trailing newline**. Preserve the listed order of
   `captured_lens_results`, comma-joined; an empty list becomes an empty value.
   Use the row's boolean for `findings_present` and the same trimmed actor and
   reason as the command flags.

```text
gentle-ai.review-abandon-authorization/v2
lineage=<lineage_id>
revision=<revision>
snapshot_identity=<snapshot_identity>
reason=<reason>
captured_lens_results=<comma-joined captured_lens_results>
findings_present=<true or false>
actor=<actor>
```

Pass that exact multiline string as one quoted argument (`authorization` below
is a shell variable containing the filled binding):

```sh
gentle-ai review abandon --cwd <repo> \
  --lineage <lineage_id> --expected-revision <revision> \
  --reason operator_disposition --actor <actor> \
  --maintainer-authorization "$authorization"
```

Use `--reason retired_schema` instead when that is your chosen disposition.
`gentle-ai review abandon --cwd <repo>` with no required inputs prints the
binding template and field lookup instructions; it does not authorize or
perform abandonment. `gentle-ai review abandon --help` describes the flags.

## Fail-closed safety model

- Eligibility comes from persisted bytes and store topology, not a newly
  computed live-worktree target. Staleness alone is not authorization.
- The gate re-derives the discarded-work summary and checks the exact lineage,
  revision, snapshot identity, reason, and actor binding. Missing, mismatched,
  or stale authorization is refused; inspect again rather than bypassing it.
- Terminal and superseded lineages are refused. Abandonment is not approval,
  acknowledgement, or permission to deliver the discarded candidate.
- Successful abandonment quarantines the entry and persists an audit record
  bound to the discarded work. An exact replay converges idempotently.
- On partial failure, the command exits non-zero and may still emit prepared
  audit-record JSON with a quarantine path. Retain that output for
  reconciliation; do not interpret it as success or delete storage manually.
