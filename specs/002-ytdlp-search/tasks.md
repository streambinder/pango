# Tasks: YouTube Search through yt-dlp

**Input**: Design documents from `/specs/002-ytdlp-search/`

**Prerequisites**: plan.md (required), spec.md (required)

**Tests**: The constitution requires unit tests next to the code
and a 90 percent coverage gate. The task list includes the tests.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to
- Include exact file paths in descriptions

## Phase 1: Artifacts

**Purpose**: Close the specify and plan steps of cycle 002.

- [x] T001 Record the hypothesis investigation and the
      requirements in `specs/002-ytdlp-search/spec.md`
- [x] T002 Record the subprocess design and the dropped-field
      decisions in `specs/002-ytdlp-search/plan.md`

## Phase 2: Provider (Stories 1, 2 and 3)

**Purpose**: The search runs through yt-dlp and keeps its
ranking apart from the two documented drops.

- [x] T003 [US1] [US2] [US3] Expose the search in
      `sys/cmd/youtubedl.go` as `YouTubeDlSearch`, with
      fake-binary tests in `sys/cmd/youtubedl_test.go`, and
      rework the search in `provider/youtube.go` onto it:
      yt-dlp subprocess with an argument slice, JSON lines
      mapping onto `youTubeResult`, year gate and
      official-artist field dropped, verified-only channel
      score
- [x] T004 [US1] [US2] [US3] Rework `provider/youtube_test.go`
      onto a fake binary on PATH: compliant and
      non-compliant lines, empty output, non-zero exit,
      missing binary, live test gated on `TEST_YTDLP`
- [x] T005 [US1] [US3] Rework the Search-level tests in
      `provider/provider_test.go` onto the stub and remove the
      HTTP helpers that no test uses anymore

## Phase 3: Command Harness and Docs (Stories 1 and 3)

**Purpose**: The synchronization tests script the search
through the fake binary, and the docs describe the new role
of yt-dlp.

- [x] T006 [US1] [US3] Teach the fake yt-dlp in
      `cmd/harness_test.go` to answer searches with canned
      JSON lines, translate `providerErr` and `noResults` into
      control files in `syncEnv`, and remove the YouTube page
      fixtures that no code reads anymore
- [x] T007 [P] [US1] Note the search role of yt-dlp in
      `docs/installation.md`
- [x] T008 [P] [US1] Rename the live test in
      `.github/workflows/scrape.yml` and install the standalone
      yt-dlp binary before running it

## Phase 4: Verification

**Purpose**: Prove the cycle against the constitution gates.

- [x] T009 Run gofmt, gofumpt, `go vet`, the full test suite
      with the race detector, and the coverage gate locally
- [x] T010 Verify live: build the binary and run the new
      search path against the four captcha-failing tracks,
      confirming the top match of each is the correct official
      video
- [x] T011 Open the pull request with spec and implementation
      in one commit and reach a green pipeline

## Phase 5: Converge

**Purpose**: Close the cycle as a historical record.

- [x] T012 Mark the task outcomes and the success criteria in
      this file at the end of the implementation
