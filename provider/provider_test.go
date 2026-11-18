package provider

import (
	"errors"
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

func BenchmarkProvider(b *testing.B) {
	for i := 0; i < b.N; i++ {
		TestSearch(&testing.T{})
	}
}

func TestSearch(t *testing.T) {
	stubYouTubeDl(t, youTubeDlScript(resultJSON))

	// testing
	matches, err := Search(track)
	assert.Nil(t, err)
	assert.NotEmpty(t, matches)
}

func TestSearchFailure(t *testing.T) {
	stubYouTubeDl(t, "#!/bin/sh\necho ko >&2\nexit 1\n")

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
	stubYouTubeDl(t, youTubeDlScript(resultJSON))

	// one provider succeeded → return its matches, no error
	matches, err := Search(track)
	assert.Nil(t, err)
	assert.NotEmpty(t, matches)
}
