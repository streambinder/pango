# Implementation Plan: YouTube Search through yt-dlp

**Branch**: `feat/ytdlp-search` | **Date**: 2026-11-18 |
**Spec**: `specs/002-ytdlp-search/spec.md`

**Input**: Feature specification from
`/specs/002-ytdlp-search/spec.md`

## Summary

The YouTube provider stops fetching the HTML search page and
runs yt-dlp instead. `search()` builds the query exactly as
today, acquires the existing two-slot semaphore, and calls
`sys/cmd.YouTubeDlSearch`, which runs
`yt-dlp --dump-json --flat-playlist --no-download
--no-warnings "ytsearch10:<query>"` through
`exec.CommandContext` with an argument slice. Each output line
is one JSON video; the fields map onto the existing
`youTubeResult` struct, with the duration and the view count
arriving as native integers. The `year` field and the
`officialArtistChannel` field leave the struct, the year gate
leaves `compliant()`, and `channelScore()` becomes
verified-only. Everything else in the provider stays: the
query, the compliance words, the score weights, the sort by
score, and the error bubbling of cycle 282. The command is
invoked by bare name, and the tests stub it with a fake yt-dlp
binary on PATH, the same way the command tests stub their
tools. The nightly live test moves from the scraper to the
real binary.

## Technical Context

**Language/Version**: Go, version from `go.mod`

**Primary Dependencies**: the standard library plus the
yt-dlp runtime binary, already a documented prerequisite;
no new dependency

**Storage**: no new storage; the search has no cache

**Testing**: fake-binary tests for the provider, the existing
Search-level tests reworked onto the fake binary, command
tests through the harness fake binary, and a live test gated
on `TEST_YTDLP`

**Constraints**: per-package coverage at 90 percent; unit
tests stub every network dependency and the yt-dlp binary;
the exported surface of the `provider` package stays
compatible

## Constitution Check

- Honest Reporting: an empty search still means not found,
  and a failing yt-dlp still surfaces its error through the
  bubbling of cycle 282 instead of a generic message.
- Graceful Enrichment: a failing search still drops only the
  track, never the synchronization.
- Library Surface: `provider.Search` keeps its signature.
  The ranking change is the two documented drops, and the
  official-artist drop loses nothing in practice because the
  badge never appears in search data.
- Specification Cycle: this document and the task list
  complete the plan and tasks steps of cycle 002. Spec and
  implementation land in one pull request as one commit, on
  the owner's order.

## Decisions

- **Subprocess over a new HTTP client**: the scrape is the
  failure, not the HTTP layer. yt-dlp speaks the InnerTube
  API, which does not serve the captcha interstitial, and it
  is already a runtime dependency for downloads.
- **Flat playlist over full extraction**: full extraction
  returns the upload date, but at about fourteen seconds per
  result. The year gate is dropped instead, which matches
  the old behavior when the date was unknown.
- **Description kept to its first line**: the scraper read
  the snippet, the opening lines of the description, while
  yt-dlp returns the full description. The credits in the
  full text carry misleading words like "studio", which
  systematically penalize official videos; the first line
  approximates the snippet and keeps the ranking the
  before/after comparison proved.
- **Official-artist badge dropped, not replaced**: the badge
  never appears in search data (0 occurrences across three
  queries), so the check was dead code. `AUDIO_BADGE` is the
  marker the official videos actually carry, and it is the
  candidate if a future change wants a channel signal from
  search data; this cycle does not use it.
- **Verified-only channel score, maximum 30**: the verified
  weight stays as it was. No re-weighting: the official part
  was dead, so the effective score is unchanged.
- **Plain command, no Android client**: the Android variant
  returns correct results, but whether it changes the search
  client is unproven. The plain command is the adopted one.
- **No retry loop in the provider**: the circuit breaker and
  the nightly runs cover recovery, as they did for the
  scrape.
- **Bare name, fake binary on PATH**: the search shells out
  to `yt-dlp` by bare name, and the tests stub it with a fake
  binary on PATH, the same way the command tests drive their
  tools.
- **Harness translation in `syncEnv`**: the `providerErr`
  and `noResults` script flags keep their meaning and now
  drive the fake yt-dlp through control files, since the
  search no longer touches HTTP.
- **Live test renamed**: `TestScraping` becomes
  `TestYouTubeSearchLive`, gated on `TEST_YTDLP`, and the
  nightly workflow installs the standalone yt-dlp binary.

## Project Structure

### Documentation (this feature)

```text
specs/002-ytdlp-search/
├── spec.md
├── plan.md
└── tasks.md
```

### Source Code (repository root)

```text
sys/cmd/youtubedl.go      # yt-dlp search command
sys/cmd/youtubedl_test.go # search command tests on a fake binary
provider/youtube.go       # search through sys/cmd, JSON lines mapping, dropped fields
provider/youtube_test.go  # fake-binary tests, live test
provider/provider_test.go # Search-level tests on the fake binary, unused HTTP helpers removed
cmd/harness_test.go       # fake yt-dlp answers searches, syncEnv drives control files
docs/installation.md     # yt-dlp now searches as well as fetches
.github/workflows/scrape.yml # live test name, yt-dlp install step
```
