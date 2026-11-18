package provider

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/streambinder/pango/entity"
	"github.com/streambinder/pango/sys"
	"github.com/stretchr/testify/assert"
)

const (
	// resultJSON is a compliant search result for the fixture track:
	// title and owner match, duration matches, verified channel. Its
	// description carries a misleading word, so the description score
	// exercises the misleading penalty.
	resultJSON = `{"id":"abc123","title":"Title","uploader":"Artist",` +
		`"description":"some description cover","duration":180,` +
		`"view_count":1000000,"channel_is_verified":true}`
	// otherJSON would score high on duration and views, but its owner
	// and title match neither the artist nor the song: the compliance
	// filter must drop it before scoring.
	otherJSON = `{"id":"xyz999","title":"Other Song","uploader":"Someone Else",` +
		`"description":"some other description","duration":180,` +
		`"view_count":5000000,"channel_is_verified":true}`
)

// youTubeDlScript renders a stub yt-dlp that prints the given JSON
// lines, one per line, whatever its arguments.
func youTubeDlScript(lines ...string) string {
	script := "#!/bin/sh\n"
	for _, line := range lines {
		script += fmt.Sprintf("printf '%%s\\n' '%s'\n", line)
	}
	return script
}

// stubYouTubeDl replaces the yt-dlp binary with a stub script on PATH
// for the duration of the test, the same way the command tests stub
// their tools.
func stubYouTubeDl(t *testing.T, script string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "ytdlp-stub-*")
	assert.Nil(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "yt-dlp")
	assert.Nil(t, os.WriteFile(path, []byte(script), 0o600))
	//nolint:gosec // the stub must run as a command.
	assert.Nil(t, os.Chmod(path, 0o755))
	t.Setenv("PATH", dir)
}

func BenchmarkYouTube(b *testing.B) {
	for i := 0; i < b.N; i++ {
		TestYouTubeSearch(&testing.T{})
	}
}

func TestYouTubeSearch(t *testing.T) {
	stubYouTubeDl(t, youTubeDlScript(resultJSON, otherJSON))

	// testing: only the compliant result is returned, with its score
	// (description 16 + duration 30 + views 12 + channel 4, the
	// description carrying the misleading penalty)
	matches, err := youTube{}.search(track)
	assert.Nil(t, err)
	assert.Len(t, matches, 1)
	assert.Equal(t, "https://youtu.be/abc123", matches[0].URL)
	assert.Equal(t, 62, matches[0].Score)
}

func TestYouTubeSearchEmptyOutput(t *testing.T) {
	stubYouTubeDl(t, youTubeDlScript())

	// testing: no lines means no matches, not an error
	matches, err := youTube{}.search(track)
	assert.Nil(t, err)
	assert.Empty(t, matches)
}

func TestYouTubeSearchDescriptionFirstLine(t *testing.T) {
	// the description is the opening lines only: a misleading word
	// in the credits below the first line must not penalize the
	// result, the way the scraper snippet never saw it
	stubYouTubeDl(t, youTubeDlScript(
		`{"id":"abc123","title":"Title","uploader":"Artist",`+
			`"description":"some description\ncover in the credits","duration":180,`+
			`"view_count":1000000,"channel_is_verified":true}`,
	))

	// testing: no misleading penalty, the full score
	matches, err := youTube{}.search(track)
	assert.Nil(t, err)
	assert.Len(t, matches, 1)
	assert.Equal(t, 86, matches[0].Score)
}

func TestYouTubeSearchMalformedLine(t *testing.T) {
	stubYouTubeDl(t, youTubeDlScript("not json"))

	// testing: a line that is not JSON is a format problem, surfaced
	// as an error instead of silently reporting the track as not found
	assert.ErrorContains(t, sys.ErrOnly(youTube{}.search(track)), "cannot parse search result")
}

func TestYouTubeSearchFailingBinary(t *testing.T) {
	stubYouTubeDl(t, "#!/bin/sh\necho ko >&2\nexit 1\n")

	// testing: a non-zero exit bubbles the yt-dlp output up
	err := sys.ErrOnly(youTube{}.search(track))
	assert.ErrorContains(t, err, "yt-dlp search failed")
	assert.ErrorContains(t, err, "ko")
}

func TestYouTubeSearchFailingBinarySilent(t *testing.T) {
	stubYouTubeDl(t, "#!/bin/sh\nexit 1\n")

	// testing: a non-zero exit with no output still reports the exit
	err := sys.ErrOnly(youTube{}.search(track))
	assert.ErrorContains(t, err, "yt-dlp search failed")
	assert.ErrorContains(t, err, "exit status 1")
}

func TestYouTubeSearchMissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	// testing: a missing binary is a clear error, not a silent miss
	assert.ErrorContains(t, sys.ErrOnly(youTube{}.search(track)), "yt-dlp binary not found")
}

func TestYouTubeSearchLive(t *testing.T) {
	if os.Getenv("TEST_YTDLP") == "" {
		t.Skip("TEST_YTDLP unset")
	}
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		t.Skip("yt-dlp binary not found in PATH")
	}

	// testing: the real binary searches live
	matches, err := youTube{}.search(&entity.Track{
		Title:    "White Christmas",
		Artists:  []string{"Bing Crosby"},
		Duration: 183,
	})
	assert.Nil(t, err)
	assert.NotEmpty(t, matches)
}
