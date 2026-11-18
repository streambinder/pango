package provider

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/streambinder/pango/entity"
	"github.com/stretchr/testify/assert"
)

var track = &entity.Track{
	ID:       "123",
	Title:    "Title",
	Artists:  []string{"Artist"},
	Album:    "Album",
	Artwork:  entity.Artwork{URL: "http://ima.ge"},
	Duration: 180,
	Number:   1,
	Year:     1970,
}

// roundTripFunc adapts a function to http.RoundTripper, so tests can
// script every HTTP response the providers receive without any network.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

// stubHTTP routes all provider HTTP traffic through the given handler:
// the shared default transport is replaced for the duration of the test.
func stubHTTP(t *testing.T, handler func(*http.Request) (*http.Response, error)) {
	t.Helper()
	previousDefault := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(handler)
	t.Cleanup(func() {
		http.DefaultTransport = previousDefault
	})
}

// failingProvider is a provider whose search always fails, to exercise
// the partial failure path of Search alongside a working provider.
type failingProvider struct{}

func (failingProvider) search(*entity.Track) ([]*Match, error) {
	return nil, errors.New("ko")
}

// errProvider is a provider whose search always fails with the given
// error, to exercise error bubbling in Search.
type errProvider struct{ err error }

func (p errProvider) search(*entity.Track) ([]*Match, error) {
	return nil, p.err
}

func httpResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// errReader fails every read, to exercise body reading failures.
type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }
func (r errReader) Close() error             { return nil }

func BenchmarkProvider(b *testing.B) {
	for i := 0; i < b.N; i++ {
		TestSearch(&testing.T{})
	}
}

func TestSearch(t *testing.T) {
	stubHTTP(t, func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "www.youtube.com" {
			return httpResponse(200, youtubeResultPage()), nil
		}
		return nil, errors.New("unexpected host: " + request.URL.Host)
	})

	// testing
	matches, err := Search(track)
	assert.Nil(t, err)
	assert.NotEmpty(t, matches)
}

func TestSearchFailure(t *testing.T) {
	stubHTTP(t, func(*http.Request) (*http.Response, error) {
		return nil, errors.New("ko")
	})

	// all providers failed → propagate as error, carrying the
	// downstream cause instead of a generic message
	_, err := Search(track)
	assert.ErrorContains(t, err, "all providers failed")
	assert.ErrorContains(t, err, "ko")
}

func TestSearchFailureBubblesUpstreamErrors(t *testing.T) {
	previous := providers
	providers = []Provider{
		errProvider{errors.New("youtube: blocked by google captcha")},
		errProvider{errors.New("second provider: connection refused")},
	}
	t.Cleanup(func() { providers = previous })

	// testing: every provider failed → the error carries every
	// downstream cause, not a generic message
	_, err := Search(track)
	assert.ErrorContains(t, err, "all providers failed")
	assert.ErrorContains(t, err, "youtube: blocked by google captcha")
	assert.ErrorContains(t, err, "second provider: connection refused")
}

func TestSearchPartialFailure(t *testing.T) {
	previous := providers
	providers = append(append([]Provider{}, providers...), failingProvider{})
	t.Cleanup(func() { providers = previous })
	stubHTTP(t, func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "www.youtube.com" {
			return httpResponse(200, youtubeResultPage()), nil
		}
		return nil, errors.New("unexpected host: " + request.URL.Host)
	})

	// one provider succeeded → return its matches, no error
	matches, err := Search(track)
	assert.Nil(t, err)
	assert.NotEmpty(t, matches)
}
