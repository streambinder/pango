# Implementation Plan: Dry-Run Flag for Unsync

**Branch**: `feat/unsync` | **Date**: 2026-11-26 |
**Spec**: `specs/005-unsync-dry-run/spec.md`

**Input**: Feature specification from
`/specs/005-unsync-dry-run/spec.md`

**Depends on**: Cycle 004, spec
`specs/004-unsync-subcommand-deletes/spec.md`, implemented
on branch `feat/unsync` in PR #286, open and unmerged.
This cycle adds a flag to the command that cycle 004
introduces.

## Summary

A `--dry-run` boolean flag on the `unsync` command runs
every read stage identically to a real run — index build,
Spotify authentication, and fetch of every requested
collection — and skips only the removal calls. For each
fetched track the command still resolves the expected
local file from `Track.Path().Final()`, reads the stored
Spotify identifier from the "Spotify ID" frame, and
reports "would delete" only on an identifier match. The
deduplication map still records would-be-deleted
identifiers, so a track in two collections is counted
once. For each requested playlist the command reports the
`.m3u` and `.pls` files as would-be-removed. No `os.Remove`
call runs, and no `Index.Delete` call runs: the run leaves
every state untouched, in memory as on disk. The summary
reuses the same counters with "would be" wording. The exit
code is zero whenever the read stages succeed, because no
deletion is attempted and no deletion can fail. With no
collection flag the command still prints that there is
nothing to do and exits zero before any work.

## Technical Context

**Language/Version**: Go, version from `go.mod`

**Primary Dependencies**: none new; the flag, the index,
the ID3 layer, and the Spotify client already exist

**Storage**: no new storage; a dry run writes nothing to
the library and mutates no in-memory index entry

**Testing**: command tests through the existing harness in
`cmd/harness_test.go` (`syncEnv`, `testExecute`,
`writeTaggedFile`, scripted Spotify transport); no test
calls a live provider

**Constraints**: per-package coverage at 90 percent; unit
tests stub every network dependency; the exported surface
of the `entity`, `spotify`, and `sys` packages is a
compatibility contract, so this cycle adds no exported
symbol; no live verification of the full library before
the merge

## Constitution Check

- Metadata Is the Index: the match rule is unchanged. A
  dry run reads the same ID3 frames that the processor
  writes and that `show` and `audit` read. It writes no
  frame and deletes no file, so the index of a later run
  is identical to the index before the dry run.
- Pipeline Shape: `unsync` adds no pipeline stage. The
  flag gates two call sites inside the existing delete
  step. Each step keeps its failure policy local: a fetch
  failure aborts before any would-be deletion is reported,
  and the exit code is zero when the read stages succeed.
- Honest Reporting: the report distinguishes "would
  delete" from "deleted", a skipped file from an absent
  file, and a would-be-deleted duplicate from a first
  occurrence. A fetch failure never looks like a completed
  preview, because the fetch completes before any report.
  The stated limit is part of the report contract: a dry
  run cannot predict a deletion failure, so "would delete"
  is a match statement, never a promise of success.
- Graceful Enrichment: not applicable. `unsync` collects
  no enrichment.
- Library Surface: no exported signature changes. The
  flag is command-local.
- Minimal Owned Surface: no new dependency. The change
  gates standard-library calls.
- Production Care: the change is verified on a bounded
  selection before it is declared ready, never on a first
  run against the full library. The live ponz dry run is
  the owner's, after the merge and the deploy.
- Specification Cycle: this document and the task list
  complete the plan and tasks steps of cycle 005. The
  specification is a draft until the owner ratifies it.
  Implementation follows in a later cycle.

No amendment to the constitution is needed for this cycle.
Verified on 2026-11-26 against branch `feat/unsync`,
commit `b603317`: the constitution names no dry-run
concept, the provider section is untouched by this cycle,
and the architecture baseline names no component this
cycle changes. Cycles 001, 002, and 003 stay Draft and
unratified even though their changes were implemented and
merged; that anomaly is recorded in the cycle 004 report,
not repaired here, because a completed feature directory
is an immutable historical record.

## Decisions

- **Gate the two removal sites only**: a `dryRun` boolean
  threads from `RunE` into `unsyncDeleteTrack`,
  `unsyncRemoveFile`, and `unsyncDeletePlaylistFiles`.
  In dry-run mode `unsyncRemoveFile` records the
  would-be-deleted identifier, prints "would delete",
  and returns without `os.Remove` and without
  `Index.Delete`. `unsyncDeletePlaylistFiles` appends the
  filename to the playlist-files list, prints "would
  remove playlist file", and returns without `os.Remove`.
  Rejected: a separate dry-run code path (it would drift
  from the real match logic with every later change).
- **Skip Index.Delete in dry-run mode**: the in-memory
  index stays untouched, so the simulation is provably a
  pure read. The deletion path never consults the index
  after the build, so skipping the call changes no
  observable behaviour inside the run.
- **Deduplication still applies**: the `deleted` map
  records would-be-deleted identifiers in dry-run mode.
  The second occurrence of a track in two collections
  prints "already counted" and increments the
  already-deleted count. Rejected: printing "already
  deleted" in a dry run (nothing was deleted, and the
  constitution's Honest Reporting rule forbids the claim).
- **Same counters, "would be" wording**: the summary
  keeps the same five counters in the same order and
  changes only the verbs (FR-009). Rejected: a separate
  would-be-deleted counter next to the deleted count
  (it would double every downstream parsing of the
  summary for no new information).
- **Exit policy**: `results.failed` stays zero in a dry
  run, so the exit code is zero when index, auth, and
  fetch succeed. A fetch failure returns the same error
  as a real run. Rejected: probing removability before
  reporting (it would need to attempt the removal, which
  is exactly what a dry run must not do).
- **Bare unsync with dry-run still does nothing**: the
  empty-selection check runs before any work, as in spec 004.
  The flag does not wake the command.
- **No short flag**: `--dry-run` is long-only, following
  the boolean convention of `sync` (`--plain`,
  `--skip-lyrics`) and `audit` (`--library`). `-n` is free
  in every command, so the choice is convention, not a
  conflict.
- **Fold into PR #286**: the implementation amends the
  single squashed commit of `feat/unsync` and re-verifies
  the pipeline green on the new head, instead of opening
  a separate stacked PR. The flag changes the same lines
  PR #286 introduces (`RunE`, the remove helpers, the
  summary), #286 is unmerged, and one feature keeps one
  commit and one review. Rejected: a stacked PR on
  `feat/unsync` (it would add a second review round for a
  change of about one hundred lines on an unmerged
  branch, and the stacked diff would churn the same
  lines the reviewer is already reading). The decision
  stays open for ratification as Q3.

## Project Structure

### Documentation (this feature)

```text
specs/005-unsync-dry-run/
├── spec.md
├── plan.md
└── tasks.md
```

### Source Code (repository root)

```text
cmd/unsync.go      # --dry-run flag, gated removal sites, dry-run wording
cmd/unsync_test.go # harness tests: playlist, album, track, empty, failures
```
