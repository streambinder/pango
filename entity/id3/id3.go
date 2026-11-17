package id3

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"strings"
	"unicode/utf16"

	"github.com/bogem/id3v2/v2"
	"github.com/streambinder/pango/lyrics"
)

const (
	frameAttachedPicture      = "Attached picture"
	frameTrackNumber          = "Track number/Position in set"
	frameUnsynchronizedLyrics = "Unsynchronised lyrics/text transcription"
	frameUserDefinedText      = "User defined text information frame"
	frameSpotifyID            = "Spotify ID"
	frameArtworkURL           = "Artwork URL"
	frameDuration             = "Duration"
	frameUpstreamURL          = "Upstream URL"
)

type Tag struct {
	id3v2.Tag
	Cache map[string]string
}

func Open(path string, options id3v2.Options) (*Tag, error) {
	tag, err := id3v2.Open(path, options)
	if err != nil {
		return nil, err
	}
	return &Tag{*tag, make(map[string]string)}, err
}

func OpenSpotifyID(path string) (*Tag, error) {
	tag, err := Open(path, id3v2.Options{
		Parse: true,
		// Spotify ID is serialized as a user-defined text (TXXX) frame.
		ParseFrames: []string{frameUserDefinedText},
	})
	if err == nil {
		return tag, nil
	}

	// bogem reads past a tag that ends in fewer than ten bytes of
	// padding (a frame header cannot fit there) and fails with
	// "frame went over tag area", even though the frames themselves
	// are intact; fall back to a tolerant walk that stops at the
	// padding and still returns the Spotify ID
	id, lerr := readSpotifyIDLenient(path)
	if lerr != nil {
		return nil, err
	}
	return &Tag{*id3v2.NewEmptyTag(), map[string]string{frameSpotifyID: id}}, nil
}

// synchsafeSize decodes an ID3v2 synchsafe integer, four seven-bit
// bytes; it reports false when a byte carries the high bit
func synchsafeSize(b []byte) (int64, bool) {
	var size int64
	for _, c := range b {
		if c&0x80 != 0 {
			return 0, false
		}
		size = size<<7 | int64(c)
	}
	return size, true
}

// decodeText decodes frame text according to its ID3v2 text encoding:
// 0 ISO-8859-1, 1 UTF-16 with BOM, 2 UTF-16BE, 3 UTF-8
func decodeText(b []byte, encoding byte) string {
	switch encoding {
	case 1:
		if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
			return decodeUTF16(b[2:], binary.LittleEndian)
		}
		if len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF {
			return decodeUTF16(b[2:], binary.BigEndian)
		}
		return decodeUTF16(b, binary.LittleEndian)
	case 2:
		return decodeUTF16(b, binary.BigEndian)
	case 0:
		runes := make([]rune, len(b))
		for i, c := range b {
			runes[i] = rune(c)
		}
		return string(runes)
	default:
		return string(b)
	}
}

func decodeUTF16(b []byte, order binary.ByteOrder) string {
	if len(b)%2 != 0 {
		b = b[:len(b)-1]
	}
	u16 := make([]uint16, len(b)/2)
	for i := range u16 {
		u16[i] = order.Uint16(b[2*i:])
	}
	return string(utf16.Decode(u16))
}

// txxxValue returns the value of a user-defined text frame whose
// description matches key
func txxxValue(body []byte, key string) (string, bool) {
	if len(body) < 1 {
		return "", false
	}
	encoding := body[0]
	rest := body[1:]

	var desc, value []byte
	if encoding == 1 || encoding == 2 {
		// UTF-16 text is null-terminated by a two-byte unit, aligned
		// to the start of the text
		end := -1
		for i := 0; i+1 < len(rest); i += 2 {
			if rest[i] == 0 && rest[i+1] == 0 {
				end = i
				break
			}
		}
		if end < 0 {
			return "", false
		}
		desc, value = rest[:end], rest[end+2:]
	} else {
		end := bytes.IndexByte(rest, 0)
		if end < 0 {
			return "", false
		}
		desc, value = rest[:end], rest[end+1:]
	}

	if !strings.EqualFold(decodeText(desc, encoding), key) {
		return "", false
	}
	// writers such as mutagen null-terminate the value; the
	// terminator is not part of it
	return strings.Trim(decodeText(value, encoding), "\x00"), true
}

// readID3Header reads the ten-byte ID3v2 header and returns its
// version and tag size
func readID3Header(file *os.File) (byte, int64, error) {
	header := make([]byte, 10)
	if _, err := io.ReadFull(file, header); err != nil {
		return 0, 0, err
	}
	if string(header[:3]) != "ID3" {
		return 0, 0, errors.New("no ID3 tag")
	}
	version := header[3]
	if version < 3 || version > 4 {
		return 0, 0, errors.New("unsupported ID3 version")
	}
	size, ok := synchsafeSize(header[6:10])
	if !ok || size < 0 {
		return 0, 0, errors.New("invalid ID3 tag size")
	}
	return version, size, nil
}

// frameSize decodes the size of the frame at the start of tag: a
// synchsafe integer for ID3v2.4, a plain integer for ID3v2.3
func frameSize(tag []byte, version byte) (int64, bool) {
	if version == 4 {
		return synchsafeSize(tag[4:8])
	}
	return int64(binary.BigEndian.Uint32(tag[4:8])), true
}

// readSpotifyIDLenient extracts the Spotify ID from a tag by walking
// its frames by hand, stopping when fewer than ten bytes remain (a
// frame header cannot fit there) or at a blank frame, instead of
// reading past the tag area into the audio data
func readSpotifyIDLenient(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	version, size, err := readID3Header(file)
	if err != nil {
		return "", err
	}
	tag := make([]byte, size)
	if _, err := io.ReadFull(file, tag); err != nil {
		return "", err
	}

	for len(tag) >= 10 {
		id := string(tag[:4])
		if id == "\x00\x00\x00\x00" {
			break
		}
		fsize, ok := frameSize(tag, version)
		if !ok || fsize < 0 || 10+fsize > int64(len(tag)) {
			return "", errors.New("invalid frame size")
		}
		if id == "TXXX" {
			if value, found := txxxValue(tag[10:10+fsize], frameSpotifyID); found {
				return value, nil
			}
		}
		tag = tag[10+fsize:]
	}
	return "", errors.New("no Spotify ID frame")
}

func (tag *Tag) SetTrackNumber(number string) {
	tag.AddFrame(
		tag.CommonID(frameTrackNumber),
		id3v2.TextFrame{
			Encoding: tag.DefaultEncoding(),
			Text:     number,
		},
	)
}

func (tag *Tag) TrackNumber() string {
	return tag.GetTextFrame(tag.CommonID(frameTrackNumber)).Text
}

func (tag *Tag) setUserDefinedText(key, value string) {
	tag.AddUserDefinedTextFrame(id3v2.UserDefinedTextFrame{
		Encoding:    tag.DefaultEncoding(),
		Description: key,
		Value:       value,
	})
}

func (tag *Tag) userDefinedText(key string) string {
	if value, ok := tag.Cache[key]; ok {
		return value
	}

	for _, rawFrame := range tag.GetFrames(tag.CommonID(frameUserDefinedText)) {
		frame, ok := rawFrame.(id3v2.UserDefinedTextFrame)
		if !ok {
			continue
		}
		tag.Cache[frame.UniqueIdentifier()] = frame.Value

		if strings.EqualFold(frame.UniqueIdentifier(), key) {
			return frame.Value
		}
	}

	return ""
}

func (tag *Tag) SetSpotifyID(id string) {
	tag.setUserDefinedText(frameSpotifyID, id)
}

func (tag *Tag) SpotifyID() string {
	return tag.userDefinedText(frameSpotifyID)
}

func (tag *Tag) SetArtworkURL(url string) {
	tag.setUserDefinedText(frameArtworkURL, url)
}

func (tag *Tag) ArtworkURL() string {
	return tag.userDefinedText(frameArtworkURL)
}

func (tag *Tag) SetDuration(duration string) {
	tag.setUserDefinedText(frameDuration, duration)
}

func (tag *Tag) Duration() string {
	return tag.userDefinedText(frameDuration)
}

func (tag *Tag) SetUpstreamURL(url string) {
	tag.setUserDefinedText(frameUpstreamURL, url)
}

func (tag *Tag) UpstreamURL() string {
	return tag.userDefinedText(frameUpstreamURL)
}

func (tag *Tag) SetAttachedPicture(picture []byte) {
	tag.AddAttachedPicture(id3v2.PictureFrame{
		Encoding:    tag.DefaultEncoding(),
		MimeType:    "image/jpeg",
		PictureType: id3v2.PTFrontCover,
		Description: "Front cover",
		Picture:     picture,
	})
}

func (tag *Tag) AttachedPicture() (string, []byte) {
	frame, ok := tag.GetLastFrame(tag.CommonID(frameAttachedPicture)).(id3v2.PictureFrame)
	if ok {
		return frame.MimeType, frame.Picture
	}
	return "", []byte{}
}

func (tag *Tag) SetLyrics(lyrics string) {
	tag.setUnsynchronizedLyrics(lyrics)
}

func (tag *Tag) setUnsynchronizedLyrics(data string) {
	// The content descriptor stays empty: players reading tags
	// through FFmpeg expose the frame as lyrics-<descriptor>-<lang>,
	// so a descriptor hides the lyrics from players that look up
	// the plain lyrics key.
	tag.AddUnsynchronisedLyricsFrame(id3v2.UnsynchronisedLyricsFrame{
		Encoding:          tag.DefaultEncoding(),
		Language:          "eng",
		ContentDescriptor: "",
		Lyrics:            lyrics.GetPlain(data),
	})
}

func (tag *Tag) UnsynchronizedLyrics() string {
	frame, ok := tag.GetLastFrame(tag.CommonID(frameUnsynchronizedLyrics)).(id3v2.UnsynchronisedLyricsFrame)
	if ok {
		return frame.Lyrics
	}
	return ""
}

func (tag *Tag) Close() error {
	if err := tag.Tag.Close(); err != id3v2.ErrNoFile && err != nil {
		return err
	}
	return nil
}
