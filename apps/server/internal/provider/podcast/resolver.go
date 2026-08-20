package podcast

import (
	"context"
	"errors"
	"net/http"

	domain "github.com/Actify/echonote/apps/server/internal/domain/podcast"
)

type Resolver struct {
	resolvers []domain.EpisodeResolver
}

func NewResolver(client *http.Client) *Resolver {
	return &Resolver{resolvers: []domain.EpisodeResolver{
		newAppleResolver(client),
		newDirectAudioResolver(client),
		newRSSResolver(client),
	}}
}

func (r *Resolver) CanResolve(rawURL string) bool {
	for _, resolver := range r.resolvers {
		if resolver.CanResolve(rawURL) {
			return true
		}
	}
	return false
}

func (r *Resolver) Resolve(ctx context.Context, rawURL string) (*domain.ResolvedEpisode, error) {
	for _, resolver := range r.resolvers {
		if !resolver.CanResolve(rawURL) {
			continue
		}
		resolved, err := resolver.Resolve(ctx, rawURL)
		if errors.Is(err, domain.ErrNotApplicable) {
			continue
		}
		return resolved, err
	}
	return nil, domain.NewResolveError("IMPORT_UNSUPPORTED_URL", "URL is not a supported Apple Podcasts, RSS, or direct audio source", false, nil)
}
