package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/ytb-cli/ytb"

	"github.com/streambinder/pango/entity"
	"github.com/streambinder/pango/sys"
	"github.com/stretchr/testify/assert"
)

// roundTripFunc adapts a function to http.RoundTripper, so tests can
// script the responses the shared ytb client receives without any
// network.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		StatusCode:    status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": []string{"application/json"}},
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
	}
}

// youTubeStub scripts the YouTube HTTP boundary the shared client
// talks to: the embed harvest page, the InnerTube search endpoint,
// and channel pages. A search that answers 429 or 500 exercises the
// provider error path; a channel page that answers 500 exercises the
// badge-fetch degradation.
type youTubeStub struct {
	searchBody    string
	searchStatus  int
	channelStatus int
	verified      map[string]bool
}

func (stub youTubeStub) roundTrip(request *http.Request) (*http.Response, error) {
	switch {
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/embed/"):
		return jsonResponse(200, `"INNERTUBE_API_KEY":"test-key",`+
			`"INNERTUBE_CLIENT_VERSION":"2.0","VISITOR_DATA":"test-visitor"`), nil
	case request.Method == http.MethodPost && request.URL.Path == "/youtubei/v1/search":
		status := stub.searchStatus
		if status == 0 {
			status = 200
		}
		return jsonResponse(status, stub.searchBody), nil
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/channel/"):
		status := stub.channelStatus
		if status == 0 {
			status = 200
		}
		channelID := strings.Trim(strings.TrimPrefix(request.URL.Path, "/channel/"), "/")
		return jsonResponse(status, channelPageJSON(channelID, stub.verified[channelID])), nil
	}
	return jsonResponse(404, `{}`), nil
}

// stubYouTube replaces the shared client with one built with no
// pacing and no retries, talking to the stub transport, for the
// duration of the test.
func stubYouTube(t *testing.T, stub youTubeStub) {
	t.Helper()
	client := ytb.NewClient(ytb.Config{
		Workers: 1,
		Delay:   0,
		Timeout: 30 * time.Second,
		Retries: 0,
		HL:      "en",
		GL:      "US",
	})
	client.HTTP().Transport = roundTripFunc(stub.roundTrip)
	previous := sys.YouTubeClient
	sys.YouTubeClient = client
	t.Cleanup(func() { sys.YouTubeClient = previous })
}

// videoRow renders one videoRenderer row: a compliant search result
// for the fixture track unless the caller changes the fields.
func videoRow(id, title, owner, ownerID, description, length, views string) map[string]any {
	return map[string]any{
		"videoId": id,
		"title":   map[string]any{"simpleText": title},
		"descriptionSnippet": map[string]any{
			"simpleText": description,
		},
		"lengthText":        map[string]any{"simpleText": length},
		"publishedTimeText": map[string]any{"simpleText": "1 month ago"},
		"ownerText": map[string]any{"runs": []any{map[string]any{
			"text": owner,
			"navigationEndpoint": map[string]any{
				"browseEndpoint": map[string]any{"browseId": ownerID},
			},
		}}},
		"viewCountText": map[string]any{"simpleText": views},
	}
}

// lockupRow renders one lockupViewModel row: a video with no
// description at all, its channel, views and duration carried by the
// metadata rows and the thumbnail badge.
func lockupRow(id, title, owner, ownerID, duration, views string) map[string]any {
	return map[string]any{
		"contentId":   id,
		"contentType": "LOCKUP_CONTENT_TYPE_VIDEO",
		"metadata": map[string]any{"lockupMetadataViewModel": map[string]any{
			"title": map[string]any{"content": title},
			"metadata": map[string]any{"contentMetadataViewModel": map[string]any{
				"metadataRows": []any{map[string]any{"metadataParts": []any{
					map[string]any{"text": map[string]any{
						"content": owner,
						"commandRuns": []any{map[string]any{"onTap": map[string]any{
							"innertubeCommand": map[string]any{
								"browseEndpoint": map[string]any{"browseId": ownerID},
							},
						}}},
					}},
					map[string]any{"text": map[string]any{"content": views}},
					map[string]any{"text": map[string]any{"content": "1 month ago"}},
				}}},
			}},
		}},
		"contentImage": map[string]any{"thumbnailViewModel": map[string]any{
			"overlays": []any{map[string]any{"thumbnailOverlayBadgeViewModel": map[string]any{
				"thumbnailBadges": []any{map[string]any{
					"thumbnailBadgeViewModel": map[string]any{"text": duration},
				}},
			}}},
		}},
	}
}

// searchJSON wraps the given rows in an InnerTube search response.
func searchJSON(rows ...map[string]any) string {
	items := make([]any, 0, len(rows))
	for _, row := range rows {
		if _, ok := row["contentId"]; ok {
			items = append(items, map[string]any{"lockupViewModel": row})
			continue
		}
		items = append(items, map[string]any{"videoRenderer": row})
	}
	body, err := json.Marshal(map[string]any{"contents": map[string]any{
		"twoColumnSearchResultsRenderer": map[string]any{
			"primaryContents": map[string]any{
				"sectionListRenderer": map[string]any{"contents": []any{
					map[string]any{"itemSectionRenderer": map[string]any{
						"contents": items,
					}},
				}},
			},
		},
	}})
	if err != nil {
		panic(err)
	}
	return string(body)
}

// channelPageJSON renders a channel page carrying the verified badge
// when verified is set, the way FetchChannel reads it.
func channelPageJSON(channelID string, verified bool) string {
	header := map[string]any{
		"title": map[string]any{"dynamicTextViewModel": map[string]any{
			"text": map[string]any{"attachmentRuns": []any{}},
		}},
	}
	if verified {
		header["title"].(map[string]any)["dynamicTextViewModel"].(map[string]any)["text"] = map[string]any{"attachmentRuns": []any{map[string]any{"element": map[string]any{
			"type": map[string]any{"imageType": map[string]any{"image": map[string]any{
				"sources": []any{map[string]any{"clientResource": map[string]any{
					"imageName": "CHECK_CIRCLE_FILLED",
				}}},
			}}},
		}}}}
	}
	data, err := json.Marshal(map[string]any{
		"metadata": map[string]any{"channelMetadataRenderer": map[string]any{
			"externalId": channelID,
			"title":      "Artist",
		}},
		"header": map[string]any{"pageHeaderRenderer": map[string]any{
			"content": map[string]any{"pageHeaderViewModel": header},
		}},
	})
	if err != nil {
		panic(err)
	}
	return "var ytInitialData = " + string(data) + ";"
}

// resultRow is a compliant search result for the fixture track:
// title and owner match, duration matches, verified channel. Its
// description carries a misleading word, so the description score
// exercises the misleading penalty.
func resultRow() map[string]any {
	return videoRow("abc123", "Title", "Artist", "UCartist",
		"some description cover", "3:00", "1,000,000 views")
}

// otherRow would score high on duration and views, but its owner
// and title match neither the artist nor the song: the compliance
// filter must drop it before scoring.
func otherRow() map[string]any {
	return videoRow("xyz999", "Other Song", "Someone Else", "UCother",
		"some other description", "3:00", "5,000,000 views")
}

func BenchmarkYouTube(b *testing.B) {
	for i := 0; i < b.N; i++ {
		TestYouTubeSearch(&testing.T{})
	}
}

func TestYouTubeSearch(t *testing.T) {
	stubYouTube(t, youTubeStub{
		searchBody: searchJSON(resultRow(), otherRow()),
		verified:   map[string]bool{"UCartist": true},
	})

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
	stubYouTube(t, youTubeStub{searchBody: `{}`})

	// testing: no results means no matches, not an error
	matches, err := youTube{}.search(track)
	assert.Nil(t, err)
	assert.Empty(t, matches)
}

func TestYouTubeSearchDescriptionFirstLine(t *testing.T) {
	// the snippet is the opening lines of the description only: a
	// clean snippet carries no misleading word from the credits
	// below it, the way the scraper snippet never saw them
	stubYouTube(t, youTubeStub{
		searchBody: searchJSON(videoRow("abc123", "Title", "Artist", "UCartist",
			"some description", "3:00", "1,000,000 views")),
		verified: map[string]bool{"UCartist": true},
	})

	// testing: no misleading penalty, the full score
	matches, err := youTube{}.search(track)
	assert.Nil(t, err)
	assert.Len(t, matches, 1)
	assert.Equal(t, 86, matches[0].Score)
}

func TestYouTubeSearchLockupRow(t *testing.T) {
	// a lockup row carries no description: it scores from title
	// plus owner only, and is not dropped for the missing
	// description
	stubYouTube(t, youTubeStub{
		searchBody: searchJSON(lockupRow("abc123", "Title", "Artist", "UCartist",
			"3:00", "1,000,000 views")),
		verified: map[string]bool{"UCartist": true},
	})

	// testing: the full score without a description
	matches, err := youTube{}.search(track)
	assert.Nil(t, err)
	assert.Len(t, matches, 1)
	assert.Equal(t, "https://youtu.be/abc123", matches[0].URL)
	assert.Equal(t, 86, matches[0].Score)
}

func TestYouTubeSearchUnverifiedChannel(t *testing.T) {
	// the channel page carries no verified badge: the channel part
	// of the score is zero
	stubYouTube(t, youTubeStub{
		searchBody: searchJSON(resultRow()),
		verified:   map[string]bool{},
	})

	// testing: description 16 + duration 30 + views 12, no channel part
	matches, err := youTube{}.search(track)
	assert.Nil(t, err)
	assert.Len(t, matches, 1)
	assert.Equal(t, 58, matches[0].Score)
}

func TestYouTubeSearchChannelFetchFailure(t *testing.T) {
	// a channel page that fails degrades the channel part to zero
	// and never fails the search
	stubYouTube(t, youTubeStub{
		searchBody:    searchJSON(resultRow()),
		channelStatus: 500,
	})

	// testing: the search still succeeds, without the channel part
	matches, err := youTube{}.search(track)
	assert.Nil(t, err)
	assert.Len(t, matches, 1)
	assert.Equal(t, 58, matches[0].Score)
}

func TestYouTubeSearchRateLimited(t *testing.T) {
	stubYouTube(t, youTubeStub{searchBody: `{}`, searchStatus: 429})

	// testing: a 429 past the retries is a provider error carrying
	// the rate-limit cause
	err := sys.ErrOnly(youTube{}.search(track))
	assert.ErrorContains(t, err, "youtube:")
	assert.ErrorContains(t, err, "rate limited")
}

func TestYouTubeSearchTransportFailure(t *testing.T) {
	stubYouTube(t, youTubeStub{searchBody: `{}`})
	// the transport fails every request, harvest included
	sys.YouTubeClient.HTTP().Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("ko")
	})

	// testing: a transport failure is a provider error, not a miss
	err := sys.ErrOnly(youTube{}.search(track))
	assert.ErrorContains(t, err, "youtube:")
}

func TestYouTubeSearchLive(t *testing.T) {
	if os.Getenv("TEST_YOUTUBE_LIVE") == "" {
		t.Skip("TEST_YOUTUBE_LIVE unset")
	}
	// the shared client keeps its HTTP/2 connections idle after the
	// search, and goleak fails the package on them, so close them
	t.Cleanup(func() { sys.YouTubeClient.HTTP().CloseIdleConnections() })

	// testing: the real service searches live
	matches, err := youTube{}.search(&entity.Track{
		Title:    "White Christmas",
		Artists:  []string{"Bing Crosby"},
		Duration: 183,
	})
	assert.Nil(t, err)
	assert.NotEmpty(t, matches)
}
