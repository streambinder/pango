# Tasks: YouTube Search and Download through ytb-cli

**Input**: Design documents from `/specs/003-ytb-migration/`

**Prerequisites**: plan.md (required), spec.md (required)

**Tests**: The constitution requires unit tests next to the code
and a 90 percent coverage gate. The task list includes the tests.

**Note**: This cycle covers specify, plan, and tasks only.
Implementation tasks below are unchecked and belong to a later
cycle. Do not mark a task done until its work lands and its
verification passes.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to
- Include exact file paths in descriptions

## Phase 1: Artifacts

**Purpose**: Close the specify and plan steps of cycle 003.

- [x] T001 Record the hypothesis investigation, the badge and
      description resolutions, and the requirements in
      `specs/003-ytb-migration/spec.md`
- [x] T002 Record the client, badge-fetch, download, and
      transcode design in `specs/003-ytb-migration/plan.md`

## Phase 2: Dependency (Stories 1, 3 and 4)

**Purpose**: The library is pinned and the build works with it.

- [x] T003 [US1] [US3] Run `go get github.com/tamnd/ytb-cli@v0.6.0`,
      confirm v0.6.0 is the latest stable release, move the `go`
      directive in `go.mod` to 1.26.11, and record the `go.sum`
      entries
- [x] T004 [US4] [P] Remove yt-dlp from the `Dockerfile`
      (ffmpeg only) and from `sys/cmd/environment.go`
      (ffmpeg only), with `sys/cmd/environment_test.go`
      reworked to match

## Phase 3: Search (Stories 1, 2 and 5)

**Purpose**: The provider searches through ytb and keeps its
ranking, badges included.

- [x] T005 [US1] [US2] [US5] Build the shared ytb client
      (new file only if the construction needs a home, else
      inside `provider/youtube.go`): `DefaultConfig()`, no
      session, no cache, no store, transport replaceable by
      tests
- [x] T006 [US1] [US2] Rework the search in
      `provider/youtube.go` onto `Client.Search` with
      `SearchFilters{Type: "video"}`, `PageOptions{MaxPages: 1}`,
      ten collected `Video` rows, `ErrStop` to halt, and the row
      mapping onto `youTubeResult` with the description kept to
      its first line; delete `sys/cmd/youtubedl.go` and
      `sys/cmd/youtubedl_test.go`
- [x] T007 [US2] Add the finalist badge fetch in
      `provider/youtube.go`: top three compliant results,
      one `FetchChannel` with `NoAbout: true` per distinct
      channel ID, verified-only channel score, failure
      degrading to zero, re-sort by final score
- [x] T008 [US1] [US5] Map the `NoResults` answer of
      `Client.Search` to zero matches and no error, confirming
      the detector from the fetched `kit/errs` package; every
      other search error stays a provider error
- [x] T009 [US1] [US2] [US5] Rework
      `provider/youtube_test.go` onto a stubbed HTTP
      transport: compliant and non-compliant rows, empty
      result, lockup row with no description, finalist badge
      fetch and its failure, 429 past retries, live test
      re-gated off `TEST_YTDLP`
- [x] T010 [US1] [US2] Rework the Search-level tests in
      `provider/provider_test.go` onto the stubbed transport
      and remove the yt-dlp stub helpers

## Phase 4: Download (Story 3)

**Purpose**: The chosen video downloads through ytb and
transcodes to mp3 through ffmpeg.

- [x] T011 [US3] Replace `downloader/youtubedl.go` with a ytb
      downloader: `StreamManifest`, `SelectFormat` with
      `"bestaudio/best"`, `ResolveStreamURL`,
      `DownloadToFile` with resume and manifest re-read as
      `Refresh` into a raw temporary file beside the target
- [x] T012 [US3] Add the ffmpeg transcode in `sys/cmd`
      (new file `sys/cmd/youtube.go` or beside the existing
      ffmpeg calls): transcode the raw file to the target
      extension with `-q:a 0`, captured output into the
      error, raw file removed on success and on failure, no
      partial target file left behind
- [x] T013 [US3] Rework `downloader/downloader_test.go`
      onto a stubbed transport serving a canned manifest and
      ranged bytes: download, resume, failure cleanup, and
      the existing cache-hit behaviour; remove the fake
      yt-dlp binary

## Phase 5: Command Harness, Docs, and Workflow (Stories 1, 3 and 4)

**Purpose**: The synchronization tests script the search and
download through the stubbed transport, and the docs
describe ffmpeg as the only prerequisite.

- [x] T014 [US1] [US3] Remove the fake yt-dlp binary from
      `cmd/harness_test.go` and script the search and the
      download through the stubbed transport; translate the
      `providerErr` and `noResults` script flags into
      transport behaviour in `syncEnv`
- [x] T015 [P] [US4] Note ffmpeg as the only runtime
      prerequisite in `docs/installation.md` and remove the
      yt-dlp install step from
      `.github/workflows/scrape.yml`, renaming the live test
      and its gate off `TEST_YTDLP`
- [x] T016 [P] [US4] Search the tree for `yt-dlp` outside
      `specs/` and remove every remaining reference as a
      requirement, a binary, or a test fake

## Phase 6: Verification

**Purpose**: Prove the cycle against the constitution gates,
compare against yt-dlp before the merge, and watch the first
night after it.

- [x] T017 Run gofmt, gofumpt, `go vet`, the full test suite
      with the race detector, and the coverage gate locally
- [ ] T018 Verify live before the merge, on a bounded
      selection: build the binary and run the new search and
      download paths against the four captcha-failing tracks
      of cycle 002, confirming the same top match per track,
      the same score within the verified-weight tolerance,
      and a valid mp3 of matching duration, against yt-dlp
      on the same tracks
- [ ] T019 Open the pull request and reach a green pipeline
      on the exact head before calling it ready
- [ ] T020 Observe the first overnight synchronization on
      bee after the merge: searches and downloads with no
      yt-dlp on the host, and no new failure class in the log
      against the previous nights

## Phase 7: Converge

**Purpose**: Close the cycle as a historical record.

- [x] T021 Mark the task outcomes and the success criteria in
      this file at the end of the implementation

## Phase 8: Download fallback

**Purpose**: The CDN refusal Davide reported from peregrine
recovers through the Android stream list instead of failing
the collection.

- [x] T022 [US3] On the CDN-refusal error, retry the download
      from the Android player's stream list (`FormatList`,
      best audio format with a plain URL) and restart from
      byte 0, with the fallback's own `Refresh` re-reading
      the Android list; add stubbed-transport tests for the
      fallback, its fresh start, and its failure paths, and
      record the probe results and the ANDROID_VR-first
      decision in this file

## Outcomes (fallback, 2026-11-22)

**Head**: the same single commit on `feat/ytb-migration`,
amended with the fallback, its tests, and this record.

**What the probes found, live from this VM.** The merged
manifest carries only ANDROID_VR-minted URLs for every
itag: `mergeStreams` keeps one stream per itag and the
ANDROID_VR one wins every tie, so no non-ANDROID_VR stream
exists in the manifest to fall back to — the fallback had
to come from `FormatList` (Android player) instead. A
WEB/Safari player call needs a session the bare call does
not have (`The page needs to be reloaded`), so it was not
used. The Android URL serves where the ANDROID_VR one is
refused, on the video where the two could be compared
(`dQw4w9WgXcQ`: Android 206 on both 1 MiB chunks,
ANDROID_VR 403 on both). Neither `ratebypass=yes` nor a
browser user agent lifts a refusal. The refusal varies by
video, mint, and egress: for the reported track
(`k46vgedl9os`) and for Marianne (`lWNxWy012Ro`) the CDN
refuses ranges past the first chunk on both clients' URLs
from this VM, so the fallback cannot fix those cases here.

**Decision**: ANDROID_VR stays first and the fallback runs
only on refusal. ytb itself prefers ANDROID_VR for the
download path, and the observed refusals are not specific
to that client, so preferring Android upfront would trade
one unproven bet for another. The fallback makes the
ANDROID_VR-specific refusal recoverable, which is the case
the PR body promised.

**Verification (T022)**: stubbed-transport tests cover the
fallback (ANDROID_VR 403 into an Android download that
succeeds), the fresh start (the Android host is asked from
byte 0, not from the refused stream's part file), the
no-audio and list-failure paths (the original refusal
surfaces, no partial files). Full suite green with the race
detector; coverage stays above the 90 percent floor.

**Live verification**: `dQw4w9WgXcQ` downloaded live
through the fallback (7,045,581 bytes, 213.04 seconds,
real ffmpeg transcode) — twice, the second time in the
same run as the Marianne attempt. The reported track and
Marianne still fail live from this VM (CDN 403 on both
clients), so T018 stays unchecked: search is verified on
the four cycle-002 tracks and one download is verified
live, but not the four downloads T018 asks for. The
deciding observation stays the overnight run on bee (T020),
whose egress differs from this VM and from peregrine.

## Phase 9: Download fallback pivot

**Purpose**: The first fallback (Android list) did not fix
the refusal Davide reported from peregrine, so the fallback
pivots to the client whose URLs the CDN serves.

- [x] T023 [US3] On the CDN-refusal error, retry the download
      from a VisionOS player mint first and from the Android
      player's stream list second, restarting from byte 0,
      with each stage's own `Refresh` re-minting from the
      same client; add stubbed-transport tests for both
      stages, the fall-through, and the failure paths, and
      record the per-client evidence and the pivot decision
      in this file

## Outcomes (fallback pivot, 2026-11-22)

**Head**: the same single commit on `feat/ytb-migration`,
amended with the pivot, its tests, and this record. The
amend also carries the govet shadow fix (`err` assigned,
not redeclared, in the fallback call) that CI run
45010955141 flagged on the previous head.

**The first fallback did not fix peregrine.** Davide
re-tested head `1a977a6` on peregrine (pull, forced update,
same sync command) and got the byte-identical error:
`download failure: youtube: the CDN refused this stream
URL: HTTP 403`. The Android list could not be the fix
there, and the per-client probes below say why: on the
reported track the Android URL is refused exactly like
the ANDROID_VR one.

**Per-client evidence, live from this VM, video
`k46vgedl9os`.** ANDROID_VR (manifest itag 140),
Android (`FormatList` itag 140), and iOS (player itag 140) all carry plain URLs and all fail the same way:
the first 1 MiB chunk downloads, the second is refused
with HTTP 403 (1,048,576 bytes each). The WEB and
WEB-Safari player calls fail outright (`The page needs
to be reloaded`), so there is no WEB URL to fall back
to — the WEB variant ytb can mint does not exist. The
network is ruled out: yt-dlp downloads the same track,
same itag, fully from this VM (3,177,030 bytes in 4
seconds) and from peregrine on the same network, and
yt-dlp fetching ytb's Android URL directly gets HTTP
403 — the refusal is in the minted URL, not in the
downloading client. Comparing the four URLs: none
carries an `n` or a `pot` parameter, so neither explains
it; the structural differences are the minting client
(`c=VISIONOS` for yt-dlp, `c=ANDROID_VR`/`ANDROID`/`IOS`
for ytb) and `cps` (250 against 65, absent, 145).
yt-dlp's client table says Android and iOS require a
GVS proof-of-origin token for HTTPS streaming while
VisionOS carries no such policy: the mobile clients'
URLs are minted without the token the CDN now enforces
for them, and the CDN serves the first chunk before
refusing the rest.

**Pivot**: the fallback mints as VisionOS first, through
ytb's exported `Call` with a `VISIONOS` 1.02 spec
(yt-dlp's values; ytb-cli has no such client), and
falls through to the Android list second, for the
videos VisionOS cannot serve, such as made-for-kids.
The VisionOS mint is verified live on three videos
through `DownloadToFile` (ranged, 1 MiB chunks):
`k46vgedl9os` 3,179,217 of 3,179,217 bytes,
`dQw4w9WgXcQ` 3,449,447 of 3,449,447, `lWNxWy012Ro`
3,595,264 of 3,595,264.

**Verification (T023)**: stubbed-transport tests cover
the VisionOS stage (refusal into a VisionOS download
from byte 0), the fall-through (VisionOS with no audio
into an Android download that succeeds), the no-audio
and list-failure paths on both stages (the original
refusal surfaces, no partial files), and both stages'
refreshes (two player reads each). Full suite green
with the race detector; coverage stays above the 90
percent floor.

**Live verification**: the failing track, Rick Astley,
and Marianne download end-to-end through the real
downloader with real ffmpeg transcode, from this VM:
`k46vgedl9os` 5,640,619 bytes, 196.394 seconds;
`dQw4w9WgXcQ` 7,045,581 bytes, 213.043 seconds;
`lWNxWy012Ro` 6,521,789 bytes, 222.099 seconds. That
verifies Marianne's download live, the second of the
four T018 tracks; T018 stays unchecked until the other
three downloads are verified live too. The deciding
observation stays the overnight run on bee (T020).

## Outcomes (implementation, 2026-11-22)

**Head**: one commit on `feat/ytb-migration`, author
`streambinder <automata@davidepucci.it>`, signed. T003–T017
done and verified locally; T019 (PR) and T020 (overnight on
bee) belong to the coordinator and to after the merge.

**Verification (T017)**: gofmt clean, gofumpt clean, `go vet`
clean, full suite green with the race detector. Coverage:
total 97.3%; provider 90.8%, downloader 95.3%, cmd 96.5%,
sys 98.6%, sys/cmd 100%. The committed live search test
(`TEST_YOUTUBE_LIVE`) passes; it closes the shared client's
idle connections at the end so goleak stays clean.

**Live comparison (T018, partial — left unchecked)**. Search,
run 2026-11-22 against the same-day yt-dlp baseline, same top
match on all four cycle-002 tracks, each exactly 4 points
below the old score (the verified-weight tolerance):

| Track           | yt-dlp top match | yt-dlp | ytb top match | ytb |
| --------------- | ---------------- | ------ | ------------- | --- |
| Marianne        | lWNxWy012Ro      | 74     | lWNxWy012Ro   | 70  |
| Saw You Stand   | tetYlkuHMpI      | 74     | tetYlkuHMpI   | 70  |
| Play Your Games | CJsfI1QVBEM      | 70     | CJsfI1QVBEM   | 66  |
| Tear It Down    | u7ersgydXC8      | 74     | u7ersgydXC8   | 70  |

The 4 points are the verified channel weight, lost
systematically: official artist channels carry `AUDIO_BADGE`
in place of `CHECK_CIRCLE_FILLED` in the channel header, so
`FetchChannel` reads them as unverified. The page itself was
checked to confirm this; it is YouTube's data, not a parse
error. Views arrive exact on video rows, not rounded.

Download was NOT verified end-to-end from this VM: the
manifest resolves and the ranged fetch gets HTTP 403 from
the CDN on the ANDROID_VR stream URL (first 1 MiB chunk
succeeds, later chunks and a full GET fail), while the same
video downloads fine through yt-dlp and through a WEB-client
stream URL from the same machine. The download code path
itself is covered by stubbed-transport tests (success,
resume, refresh, failure cleanup, no partial files). The
remaining verification is the overnight run on bee (T020),
where the egress differs from this VM's proxy.

**Notes**: `go get` also moved `golang.org/x/net` 0.62.0 to
0.63.0 and `golang.org/x/text` 0.43.0 to 0.44.0 transitively.
The `SelectFormat` error and empty-streams branches in
`downloader/youtube.go` stay uncovered; with at least one
stream, `"bestaudio/best"` always resolves, so they are
unreachable in practice.
