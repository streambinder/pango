package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func BenchmarkTranscode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		TestTranscode(&testing.T{})
	}
}

func TestTranscode(t *testing.T) {
	dir := installFakeBinaries(t, map[string]string{
		ffmpegName: "printf '%s\\n' \"$@\" >> \"{{DIR}}/argv\"\n" +
			"last=\"\"\nfor arg in \"$@\"; do last=\"$arg\"; done\nprintf 'audio' > \"$last\"\nexit 0\n",
	})
	raw := filepath.Join(t.TempDir(), "fname.raw")
	assert.Nil(t, os.WriteFile(raw, []byte("stream"), 0o600))
	target := filepath.Join(t.TempDir(), "fname.mp3")

	assert.Nil(t, FFmpeg().Transcode(raw, target))

	argv, err := os.ReadFile(filepath.Join(dir, "argv"))
	assert.Nil(t, err)
	assert.Contains(t, string(argv), "-i\n"+raw+"\n-q:a\n0\n")
	assert.Contains(t, string(argv), target+"\n")
	content, err := os.ReadFile(target)
	assert.Nil(t, err)
	assert.Equal(t, []byte("audio"), content)
}

func TestTranscodeFailure(t *testing.T) {
	installFakeBinaries(t, map[string]string{
		ffmpegName: printfScript("ffmpeg blew up", 1),
	})
	raw := filepath.Join(t.TempDir(), "fname.raw")
	assert.Nil(t, os.WriteFile(raw, []byte("stream"), 0o600))

	assert.ErrorContains(t,
		FFmpeg().Transcode(raw, filepath.Join(t.TempDir(), "fname.mp3")),
		"ffmpeg blew up")
}

func TestTranscodeMissingBinary(t *testing.T) {
	installEmptyPath(t)

	// a missing binary is an error, not a silent miss: the ffmpeg
	// output is empty, so the error carries the launch failure
	// through the empty output, the way the other ffmpeg calls do
	assert.NotNil(t, FFmpeg().Transcode("fname.raw", "fname.mp3"))
}
