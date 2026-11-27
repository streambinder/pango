# Implementation Plan: Unsync Subcommand Deletes Local Tracks

**Branch**: `feat/unsync` | **Date**: 2026-11-26 |
**Spec**: `specs/004-unsync-subcommand-deletes/spec.md`

**Input**: Feature specification from
`/specs/004-unsync-subcommand-deletes/spec.md`

## Summary

A new `unsync` command in `cmd/unsync.go` deletes local
tracks for the collections the owner names. The command
follows the `audit` shape, not the synchronization
pipeline: it builds the in-memory index from the output
directory, authenticates to Spotify, and fetches every
requested collection before it deletes anything. For each
fetched track it resolves the expected local file from
`Track.Path().Final()`, reads the stored Spotify identifier
from the "Spotify ID" frame, and removes the file only
when the stored identifier equals the upstream track
identifier. A deduplication set by Spotify identifier
deletes each track once across the whole run. For each
requested playlist the command then removes the
`<LegalizeFilename(Name)>.m3u` and `.pls` files. With no
collection flag the command prints that there is nothing
to do and exits zero without touching anything. A new
`Index.Delete` method drops the deleted track from the
in-memory index. The command downloads nothing and needs
no `ffmpeg`.

## Technical Context

**Language/Version**: Go, version from `go.mod`

**Primary Dependencies**: none new; the Spotify client,
the index, the ID3 layer, and the playlist encoders already
exist

**Storage**: no new storage; the index stays in memory, as
today, and deleting the file is the deletion from the
index

**Testing**: command tests through the existing harness in
`cmd/harness_test.go` (`syncEnv`, `testExecute`,
`writeTaggedFile`, scripted Spotify transport); index
tests in `entity/index/index_test.go`; no test calls a live
provider

**Constraints**: per-package coverage at 90 percent; unit
tests stub every network dependency; the exported surface
of the `entity`, `spotify`, and `sys` packages is a
compatibility contract, so the new `Index.Delete` method
is additive only; no live verification of the full library
before the merge

## Constitution Check

- Metadata Is the Index: the match rule reads the same
  ID3 frames that the processor writes and that `show`
  and `audit` read. Deleting the file removes the track
  from every later index build, because the index rebuilds
  from the files.
- Pipeline Shape: `unsync` adds no pipeline stage. It
  reuses the index, auth, and fetch stages in the `audit`
  order and then deletes. Each step keeps its failure
  policy local: a fetch failure aborts before any
  deletion, a deletion failure is reported and the run
  continues, and the exit code is non-zero at the end.
- Honest Reporting: a skipped file stays distinguishable
  from a deleted file and from an absent file. The final
  summary states the counts. A fetch failure never looks
  like a completed deletion, because the fetch completes
  before any deletion starts.
- Graceful Enrichment: not applicable. `unsync` collects
  no enrichment.
- Library Surface: `Index.Delete` is a new additive
  method. No existing exported signature changes.
- Minimal Owned Surface: no new dependency. Deletion uses
  the standard library.
- Production Care: the change is verified on a bounded
  selection before it is declared ready, never on a first
  run against the full library. The live ponz run is the
  owner's, after the merge and the deploy.
- Specification Cycle: this document and the task list
  complete the plan and tasks steps of cycle 004. The
  specification is a draft until the owner ratifies it.
  Implementation follows in a later cycle.

No amendment to the constitution is needed for this cycle.
Verified on 2026-11-26 against master `90e195f`: the
constitution names no yt-dlp and no Qobuz, the provider
section still describes the mechanism (every registered
provider is queried, the best match wins) with one
provider registered, and the architecture baseline names
no removed component. Cycles 001, 002, and 003 stay Draft
and unratified even though their changes were implemented
and merged; that anomaly is recorded in the coordinator
report, not repaired here, because a completed feature
directory is an immutable historical record.

## Decisions

- **Audit shape, not the sync pipeline**: `unsync` runs
  index, auth, fetch, then delete, sequentially, with
  plain `fmt` output like `audit`. No nursery queues, no
  TUI anchor, no `--plain` flag. Rejected: reusing the
  sync pipeline (its decide, collect, process, and install
  stages exist to download and install, which `unsync`
  never does).
- **Fetch all, then delete**: every requested collection
  is fetched before any deletion starts. A fetch failure
  aborts the run with nothing deleted. Rejected:
  interleaving fetch and delete (a late fetch failure
  would leave a half-unsynchronized library behind).
- **Match by stored identifier, not by filename**: the
  file is deleted only when the "Spotify ID" frame equals
  the upstream track identifier. The canonical filename
  only locates the candidate file. Rejected: matching by
  filename alone (two tracks can share artist and title
  with different identifiers, and the synchronization
  already treats that as a collision).
- **Deduplication by identifier**: a `map[string]bool` of
  deleted identifiers spans the whole run. A track in two
  requested collections is deleted once; the second
  occurrence reports as already deleted.
- **Playlist files deleted unconditionally on a
  successful fetch**: both `.m3u` and `.pls` go, whether
  or not the encoding flag was ever used, and whether or
  not every track matched. The playlist file is generated
  output of the collection, not user data. A missing file
  is not an error. The names come from the fetched
  playlist name, not from the flag value.
- **Deletion failure continues the run**: the failure is
  printed, the remaining tracks are processed, and the
  command returns a non-zero error at the end when any
  deletion failed. Rejected: aborting at the first failure
  (one stuck file would spare the rest of the requested
  collection for no reason).
- **No environment validation**: `unsync` never calls
  `ValidateEnvironment`. It downloads nothing and needs no
  `ffmpeg`. Rejected: calling it for symmetry with `sync`
  (it would fail a healthy machine that lacks `ffmpeg`,
  for a command that never uses it).
- **Selection read inline**: the command reads its own
  flags (`output`, `playlist`, `album`, `track`) in `RunE`
  and checks the empty selection before any work. It does
  not reuse `readSelection` from `audit.go`, because that
  helper reads the `library`, `playlist-tracks`, and
  `library-limit` flags that `unsync` does not offer.
- **No trash, no confirmation prompt**: deletion is
  permanent through `os.Remove`. The safety is the
  identifier match and the collection scope, not a prompt:
  the command runs in scripts and in the production timer
  context, and a deleted track re-synchronizes on the next
  `sync`. The prompt question stays open for ratification
  only if the owner asks for it.
- **Index.Delete removes both maps**: the method deletes
  the identifier entry and the canonical path entry of
  the track, under the index lock, so later lookups in the
  same run see the deletion.

## Project Structure

### Documentation (this feature)

```text
specs/004-unsync-subcommand-deletes/
├── spec.md
├── plan.md
└── tasks.md
```

### Source Code (repository root)

```text
cmd/unsync.go            # new command, audit shape, fetch all then delete
cmd/unsync_test.go       # harness tests: playlist, album, track, empty, failures
entity/index/index.go    # Index.Delete method (new, additive)
entity/index/index_test.go # Delete tests
```
