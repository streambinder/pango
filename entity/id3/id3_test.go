package id3

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"

	"github.com/bogem/id3v2/v2"
	"github.com/stretchr/testify/assert"
)

func BenchmarkID3(b *testing.B) {
	for i := 0; i < b.N; i++ {
		TestOpen(&testing.T{})
	}
}

// writeEmptyFile creates an empty file in a temporary directory and
// returns its path
func writeEmptyFile(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	assert.Nil(t, os.WriteFile(path, []byte{}, 0o600))
	return path
}

// writeTaggedFile creates a file holding a real ID3v2 tag with the
// given user-defined text frames and returns its path
func writeTaggedFile(t *testing.T, name string, frames map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)

	tag := id3v2.NewEmptyTag()
	for key, value := range frames {
		tag.AddUserDefinedTextFrame(id3v2.UserDefinedTextFrame{
			Encoding:    tag.DefaultEncoding(),
			Description: key,
			Value:       value,
		})
	}

	file, err := os.Create(path)
	assert.Nil(t, err)
	_, err = tag.WriteTo(file)
	assert.Nil(t, err)
	assert.Nil(t, file.Close())
	return path
}

func TestOpen(t *testing.T) {
	// testing
	tag, err := Open(writeEmptyFile(t, "track.mp3"), id3v2.Options{})
	assert.Nil(t, err)
	assert.NotNil(t, tag)
	defer func() { assert.Nil(t, tag.Close()) }()

	mimeType, image := tag.AttachedPicture()
	assert.Empty(t, mimeType)
	assert.Empty(t, image)
	assert.Empty(t, tag.UnsynchronizedLyrics())

	tag.SetAttachedPicture([]byte("picture"))
	tag.SetLyrics("lyrics")
	tag.SetLyrics("[01:01.15]lyrics")
	tag.SetTrackNumber("1")
	tag.SetSpotifyID("Spotify ID")
	tag.SetArtworkURL("Artwork URL")
	tag.SetDuration("60")
	tag.SetUpstreamURL("Upstream URL")

	mimeType, image = tag.AttachedPicture()
	assert.Equal(t, "image/jpeg", mimeType)
	assert.Equal(t, []byte("picture"), image)
	assert.Equal(t, "[01:01.15]lyrics", tag.UnsynchronizedLyrics())
	frame, ok := tag.GetLastFrame(tag.CommonID(frameUnsynchronizedLyrics)).(id3v2.UnsynchronisedLyricsFrame)
	assert.True(t, ok)
	assert.Equal(t, "", frame.ContentDescriptor)
	assert.Equal(t, "eng", frame.Language)
	assert.Equal(t, "1", tag.TrackNumber())
	assert.Equal(t, "Spotify ID", tag.SpotifyID())
	assert.Equal(t, "Artwork URL", tag.ArtworkURL())
	assert.Equal(t, "60", tag.Duration())
	assert.Equal(t, "Upstream URL", tag.UpstreamURL())
	assert.Equal(t, "Upstream URL", tag.UpstreamURL()) // served from cache
	assert.Equal(t, "", tag.userDefinedText("not existing"))
}

func TestOpenSpotifyID(t *testing.T) {
	// testing: only the user-defined text frame is parsed, any other
	// frame in the file is left out by the restricted parse options
	path := writeTaggedFile(t, "track.mp3", map[string]string{
		frameSpotifyID: "Spotify ID",
	})

	tag, err := OpenSpotifyID(path)
	assert.Nil(t, err)
	assert.NotNil(t, tag)
	defer func() { assert.Nil(t, tag.Close()) }()
	assert.Equal(t, "Spotify ID", tag.SpotifyID())
}

// rawFrame is a single ID3v2 frame written verbatim by writeRawTagFile
type rawFrame struct {
	id   string
	body []byte
}

// synchsafe encodes size as four seven-bit bytes, the ID3v2 size format
func synchsafe(size int) []byte {
	return []byte{
		byte(size >> 21 & 0x7F),
		byte(size >> 14 & 0x7F),
		byte(size >> 7 & 0x7F),
		byte(size & 0x7F),
	}
}

// plainSize encodes size as a plain big-endian integer, the ID3v2.3
// frame size format
func plainSize(size int) []byte {
	return []byte{
		byte(size >> 24),
		byte(size >> 16),
		byte(size >> 8),
		byte(size),
	}
}

// txxxBody builds a user-defined text frame body with the given
// encoding, description and value
func txxxBody(encoding byte, description, value string) []byte {
	body := []byte{encoding}
	body = append(body, description...)
	body = append(body, 0)
	body = append(body, value...)
	return body
}

// utf16Units encodes s as UTF-16 code units in the given byte order,
// without a BOM
func utf16Units(s string, order binary.ByteOrder) []byte {
	var b []byte
	for _, u := range utf16.Encode([]rune(s)) {
		var buf [2]byte
		order.PutUint16(buf[:], u)
		b = append(b, buf[:]...)
	}
	return b
}

// txxxBodyUTF16 builds a user-defined text frame body with a UTF-16
// encoding, a BOM before the description and the value, and a
// null-terminated description
func txxxBodyUTF16(encoding byte, bom []byte, order binary.ByteOrder, description, value string) []byte {
	body := []byte{encoding}
	body = append(body, bom...)
	body = append(body, utf16Units(description, order)...)
	body = append(body, 0, 0)
	body = append(body, bom...)
	body = append(body, utf16Units(value, order)...)
	return body
}

// audioAfterTag starts with bytes that decode as a huge frame size, so
// a parser reading past the tag area fails instead of stopping cleanly
var audioAfterTag = []byte{0x41, 0x42, 0x07, 0x7F, 0x7F, 0x7F, 0x00, 0x00, 0x00, 0x00, 0xFF, 0xFB, 0x90, 0x00}

// writeRawTagFile creates a file holding a hand-built ID3v2 tag with the
// given frames, trailing padding and trailing audio bytes, and returns
// its path; it lets tests reproduce tags whose frame layout a writer
// produced but a strict parser cannot read back
func writeRawTagFile(t *testing.T, name string, version byte, frames []rawFrame, padding int, audio []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)

	var tag []byte
	for _, frame := range frames {
		tag = append(tag, frame.id...)
		if version == 4 {
			tag = append(tag, synchsafe(len(frame.body))...)
		} else {
			tag = append(tag, plainSize(len(frame.body))...)
		}
		tag = append(tag, 0, 0)
		tag = append(tag, frame.body...)
	}
	tag = append(tag, make([]byte, padding)...)

	header := []byte{'I', 'D', '3', version, 0, 0}
	header = append(header, synchsafe(len(tag))...)

	assert.Nil(t, os.WriteFile(path, append(header, append(tag, audio...)...), 0o600))
	return path
}

func TestOpenSpotifyIDTrailingPadding(t *testing.T) {
	// testing: a tag ending in fewer than ten bytes of padding (a frame
	// header cannot fit there) still yields its Spotify ID instead of
	// failing with "frame went over tag area"
	tests := []struct {
		name    string
		version byte
		frames  []rawFrame
		padding int
		wantID  string
		wantErr bool
	}{
		{
			name:    "v2.4 two-byte padding",
			version: 4,
			frames:  []rawFrame{{"TXXX", txxxBody(3, frameSpotifyID, "test-id-123")}},
			padding: 2,
			wantID:  "test-id-123",
		},
		{
			name:    "v2.4 no padding",
			version: 4,
			frames:  []rawFrame{{"TXXX", txxxBody(3, frameSpotifyID, "test-id-123")}},
			padding: 0,
			wantID:  "test-id-123",
		},
		{
			name:    "v2.4 seven-byte padding",
			version: 4,
			frames:  []rawFrame{{"TXXX", txxxBody(3, frameSpotifyID, "test-id-123")}},
			padding: 7,
			wantID:  "test-id-123",
		},
		{
			name:    "v2.3 two-byte padding",
			version: 3,
			frames:  []rawFrame{{"TXXX", txxxBody(0, frameSpotifyID, "test-id-123")}},
			padding: 2,
			wantID:  "test-id-123",
		},
		{
			name:    "v2.4 two-byte padding with UTF-16LE value",
			version: 4,
			frames:  []rawFrame{{"TXXX", txxxBodyUTF16(1, []byte{0xFF, 0xFE}, binary.LittleEndian, frameSpotifyID, "test-id-123")}},
			padding: 2,
			wantID:  "test-id-123",
		},
		{
			name:    "v2.4 two-byte padding with UTF-16BE value",
			version: 4,
			frames:  []rawFrame{{"TXXX", txxxBodyUTF16(1, []byte{0xFE, 0xFF}, binary.BigEndian, frameSpotifyID, "test-id-123")}},
			padding: 2,
			wantID:  "test-id-123",
		},
		{
			name:    "v2.4 two-byte padding with UTF-16BE no-BOM value",
			version: 4,
			frames:  []rawFrame{{"TXXX", txxxBodyUTF16(2, nil, binary.BigEndian, frameSpotifyID, "test-id-123")}},
			padding: 2,
			wantID:  "test-id-123",
		},
		{
			name:    "v2.4 two-byte padding with UTF-16LE no-BOM value",
			version: 4,
			frames:  []rawFrame{{"TXXX", txxxBodyUTF16(1, nil, binary.LittleEndian, frameSpotifyID, "test-id-123")}},
			padding: 2,
			wantID:  "test-id-123",
		},
		{
			name:    "v2.4 two-byte padding with odd-length UTF-16 value",
			version: 4,
			frames:  []rawFrame{{"TXXX", append(txxxBodyUTF16(1, []byte{0xFF, 0xFE}, binary.LittleEndian, frameSpotifyID, "test-id-123"), 0x41)}},
			padding: 2,
			wantID:  "test-id-123",
		},
		{
			name:    "v2.4 two-byte padding with null-terminated value",
			version: 4,
			frames:  []rawFrame{{"TXXX", append(txxxBody(3, frameSpotifyID, "test-id-123"), 0)}},
			padding: 2,
			wantID:  "test-id-123",
		},
		{
			name:    "v2.4 two-byte padding without Spotify ID",
			version: 4,
			frames:  []rawFrame{{"TXXX", txxxBody(3, "Other", "x")}},
			padding: 2,
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeRawTagFile(t, "track.mp3", tc.version, tc.frames, tc.padding, audioAfterTag)
			tag, err := OpenSpotifyID(path)
			if tc.wantErr {
				assert.Error(t, err)
				assert.Nil(t, tag)
				return
			}
			assert.Nil(t, err)
			assert.NotNil(t, tag)
			if err != nil || tag == nil {
				return
			}
			defer func() { assert.Nil(t, tag.Close()) }()
			assert.Equal(t, tc.wantID, tag.SpotifyID())
		})
	}
}

func TestReadSpotifyIDLenientErrors(t *testing.T) {
	// testing: a tag the tolerant walk cannot read either fails
	// instead of yielding a wrong ID
	tests := []struct {
		name    string
		content []byte
	}{
		{"empty file", []byte{}},
		{"no ID3 tag", []byte("this is not an mp3 file at all")},
		{"v2.2 tag", append([]byte{'I', 'D', '3', 2, 0, 0, 0, 0, 0, 0}, make([]byte, 10)...)},
		{"bad synchsafe tag size", append([]byte{'I', 'D', '3', 4, 0, 0, 0, 0, 0, 0x80}, make([]byte, 10)...)},
		{"truncated tag", append([]byte{'I', 'D', '3', 4, 0, 0, 0, 0, 2, 0}, make([]byte, 10)...)},
		{"frame larger than tag", buildOversizedFrameTag()},
		{"frame with bad synchsafe size", buildBadFrameSizeTag()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "track.mp3")
			assert.Nil(t, os.WriteFile(path, tc.content, 0o600))
			_, err := readSpotifyIDLenient(path)
			assert.Error(t, err)
		})
	}

	// a tag with no user-defined text frame at all has no Spotify ID
	path := writeRawTagFile(t, "track.mp3", 4, []rawFrame{{"TIT2", []byte{3, 't', 'i', 't', 'l', 'e'}}}, 2, audioAfterTag)
	_, err := readSpotifyIDLenient(path)
	assert.Error(t, err)

	// a tag whose frames end in a blank frame stops at the padding
	path = writeRawTagFile(t, "track.mp3", 4, []rawFrame{{"TXXX", txxxBody(3, "Other", "x")}}, 10, audioAfterTag)
	_, err = readSpotifyIDLenient(path)
	assert.Error(t, err)

	// a file that does not exist cannot be read
	_, err = readSpotifyIDLenient(filepath.Join(t.TempDir(), "missing.mp3"))
	assert.Error(t, err)
}

// TestReadSpotifyIDLenientMalformedFrames covers user-defined text
// frames the tolerant walk cannot use
func TestReadSpotifyIDLenientMalformedFrames(t *testing.T) {
	// testing: a malformed user-defined text frame yields no ID
	// instead of a wrong one
	tests := []struct {
		name   string
		frames []rawFrame
	}{
		{"empty frame body", []rawFrame{{"TXXX", []byte{}}}},
		{"UTF-16 frame without terminator", []rawFrame{{"TXXX", append([]byte{1, 0xFF, 0xFE}, utf16Units(frameSpotifyID, binary.LittleEndian)...)}}},
		{"frame without terminator", []rawFrame{{"TXXX", append([]byte{3}, frameSpotifyID...)}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeRawTagFile(t, "track.mp3", 4, tc.frames, 2, audioAfterTag)
			_, err := readSpotifyIDLenient(path)
			assert.Error(t, err)
		})
	}
}

// buildOversizedFrameTag builds an ID3v2.4 tag holding a frame that
// claims 1000 body bytes in a 20-byte tag
func buildOversizedFrameTag() []byte {
	tag := append([]byte("TXXX"), synchsafe(1000)...)
	tag = append(tag, 0, 0)
	tag = append(tag, make([]byte, 10)...)
	header := []byte{'I', 'D', '3', 4, 0, 0}
	header = append(header, synchsafe(len(tag))...)
	return append(header, tag...)
}

// buildBadFrameSizeTag builds an ID3v2.4 tag holding a frame whose size
// carries the high bit
func buildBadFrameSizeTag() []byte {
	tag := append([]byte("TXXX"), 0, 0, 0, 0x80)
	tag = append(tag, 0, 0)
	tag = append(tag, make([]byte, 10)...)
	header := []byte{'I', 'D', '3', 4, 0, 0}
	header = append(header, synchsafe(len(tag))...)
	return append(header, tag...)
}

func TestUserDefinedTextInvalidFrame(t *testing.T) {
	// testing: a non-UserDefinedTextFrame stored under the user-defined
	// text frame ID is skipped while scanning the frames
	rawTag := id3v2.NewEmptyTag()
	rawTag.AddFrame(rawTag.CommonID(frameUserDefinedText), id3v2.TextFrame{
		Encoding: rawTag.DefaultEncoding(),
		Text:     "invalid",
	})
	tag := &Tag{*rawTag, make(map[string]string)}

	assert.Equal(t, "", tag.userDefinedText("nonexistent"))
}

func TestOpenFailure(t *testing.T) {
	// testing: opening a file that does not exist fails
	tag, err := Open(filepath.Join(t.TempDir(), "missing.mp3"), id3v2.Options{})
	assert.Nil(t, tag)
	assert.Error(t, err)
}

func TestClose(t *testing.T) {
	// testing
	tag, err := Open(writeEmptyFile(t, "track.mp3"), id3v2.Options{})
	assert.Nil(t, err)
	assert.Nil(t, tag.Close())
}

func TestCloseFailure(t *testing.T) {
	// testing: closing an already closed file-backed tag surfaces the
	// underlying file error
	tag, err := Open(writeEmptyFile(t, "track.mp3"), id3v2.Options{})
	assert.Nil(t, err)
	assert.Nil(t, tag.Close())
	assert.Error(t, tag.Close())
}

func TestCloseErrNoFile(t *testing.T) {
	// testing: closing a tag that was never initialized with a file is
	// not an error
	tag := &Tag{*id3v2.NewEmptyTag(), make(map[string]string)}
	assert.Nil(t, tag.Close())
}
