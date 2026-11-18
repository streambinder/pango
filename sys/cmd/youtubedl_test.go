package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func BenchmarkYouTubeDl(b *testing.B) {
	for i := 0; i < b.N; i++ {
		TestYouTubeDlDownload(&testing.T{})
	}
}

func TestYouTubeDlDownload(t *testing.T) {
	dir := installFakeBinaries(t, map[string]string{
		ytdlpName: "printf '%s\\n' \"$@\" >> \"{{DIR}}/argv\"\nexit 0\n",
	})

	assert.Nil(t, YouTubeDl(context.TODO(), "http://localhost", "fname.txt"))

	argv, err := os.ReadFile(filepath.Join(dir, "argv"))
	assert.Nil(t, err)
	assert.Contains(t, string(argv), "--output\nfname.%(ext)s\n")
	assert.Contains(t, string(argv), "http://localhost\n")
}

func TestYouTubeDlDownloadFailure(t *testing.T) {
	installFakeBinaries(t, map[string]string{
		ytdlpName: printfScript("yt-dlp blew up", 1),
	})

	// a partial download left in the working directory must be cleaned up
	dir := t.TempDir()
	previous, err := os.Getwd()
	assert.Nil(t, err)
	assert.Nil(t, os.Chdir(dir))
	defer func() { assert.Nil(t, os.Chdir(previous)) }()
	assert.Nil(t, os.WriteFile(filepath.Join(dir, "fname.txt"), []byte("partial"), 0o600))

	assert.ErrorContains(t,
		YouTubeDl(context.TODO(), "http://localhost", "fname.txt"),
		"yt-dlp blew up")
	_, err = os.Stat(filepath.Join(dir, "fname.txt"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestYouTubeDlSearch(t *testing.T) {
	dir := installFakeBinaries(t, map[string]string{
		ytdlpName: "printf '%s\\n' \"$@\" >> \"{{DIR}}/argv\"\nprintf '%s\\n' '{\"id\":\"abc123\"}'\nexit 0\n",
	})

	output, err := YouTubeDlSearch(context.TODO(), "Title Artist")
	assert.Nil(t, err)
	assert.Contains(t, string(output), `"id":"abc123"`)

	argv, err := os.ReadFile(filepath.Join(dir, "argv"))
	assert.Nil(t, err)
	assert.Contains(t, string(argv), "--dump-json\n--flat-playlist\n--no-download\n--no-warnings\nytsearch10:Title Artist\n")
}

func TestYouTubeDlSearchFailure(t *testing.T) {
	installFakeBinaries(t, map[string]string{
		ytdlpName: printfScript("yt-dlp blew up", 1),
	})

	_, err := YouTubeDlSearch(context.TODO(), "Title Artist")
	assert.ErrorContains(t, err, "yt-dlp search failed")
	assert.ErrorContains(t, err, "yt-dlp blew up")
}

func TestYouTubeDlSearchFailureSilent(t *testing.T) {
	installFakeBinaries(t, map[string]string{
		ytdlpName: printfScript("", 1),
	})

	_, err := YouTubeDlSearch(context.TODO(), "Title Artist")
	assert.ErrorContains(t, err, "yt-dlp search failed")
	assert.ErrorContains(t, err, "exit status 1")
}

func TestYouTubeDlSearchMissingBinary(t *testing.T) {
	installEmptyPath(t)

	_, err := YouTubeDlSearch(context.TODO(), "Title Artist")
	assert.ErrorContains(t, err, "yt-dlp binary not found in PATH")
}
