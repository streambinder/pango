package provider

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/arunsworld/nursery"
	"github.com/streambinder/pango/entity"
)

var (
	providers  = []Provider{}
	misleading = []string{"cover", "live", "karaoke", "performance", "studio", "instrumental", "remix", "acoustic"}
)

type Match struct {
	URL   string
	Score int
}

type Provider interface {
	search(track *entity.Track) ([]*Match, error)
}

func Search(track *entity.Track) ([]*Match, error) {
	var (
		workers []nursery.ConcurrentJob
		matches []*Match
		errs    []error
		mu      sync.Mutex
	)
	for _, provider := range providers {
		workers = append(workers, func(p Provider) func(ctx context.Context, ch chan error) {
			return func(_ context.Context, _ chan error) {
				scopedMatches, err := p.search(track)
				if err != nil {
					mu.Lock()
					errs = append(errs, err)
					mu.Unlock()
					return
				}
				mu.Lock()
				matches = append(matches, scopedMatches...)
				mu.Unlock()
			}
		}(provider))
	}

	if err := nursery.RunConcurrently(workers...); err != nil {
		return nil, err
	}

	// every provider failed: bubble up the downstream errors instead
	// of hiding them behind a generic message, so the error log shows
	// why the search failed
	if len(errs) == len(workers) {
		if len(errs) == 0 {
			return nil, errors.New("all providers failed")
		}
		return nil, fmt.Errorf("all providers failed: %w", errors.Join(errs...))
	}

	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	return matches, nil
}
