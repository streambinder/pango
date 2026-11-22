package cmd

import (
	"bytes"
	"errors"
	"os/exec"
)

// Transcode converts the raw downloaded stream at rawPath into the
// target audio file at targetPath, with the best VBR quality, the
// same quality the previous downloader applied with --audio-quality 0.
// output is captured into the error, the way LoudnessDetect and
// LoudnessNormalize report failures.
func (FFmpegCmd) Transcode(rawPath, targetPath string) error {
	var (
		output bytes.Buffer
		cmd    = exec.Command(
			"ffmpeg",
			"-i", rawPath,
			"-q:a", "0",
			"-y", targetPath,
		)
	)
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return errors.New(output.String())
	}
	return nil
}
