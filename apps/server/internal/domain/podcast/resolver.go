package podcast

import (
	"context"
	"errors"
	"time"
)

const (
	SourceApple       = "apple_podcasts"
	SourceRSS         = "rss"
	SourceDirectAudio = "direct_audio"
)

var ErrNotApplicable = errors.New("resolver not applicable")

type ResolvedEpisode struct {
	SourceType         string
	ExternalID         string
	RSSGUID            string
	PodcastTitle       string
	PodcastAuthor      string
	PodcastDescription string
	PodcastCoverURL    string
	EpisodeTitle       string
	Description        string
	PublishedAt        *time.Time
	DurationMS         int64
	CanonicalURL       string
	FeedURL            string
	AudioURL           string
}

type EpisodeResolver interface {
	CanResolve(rawURL string) bool
	Resolve(context.Context, string) (*ResolvedEpisode, error)
}

type ResolveError struct {
	code      string
	message   string
	retryable bool
	cause     error
}

func NewResolveError(code, message string, retryable bool, cause error) *ResolveError {
	return &ResolveError{code: code, message: message, retryable: retryable, cause: cause}
}

func (e *ResolveError) Error() string {
	return e.message
}

func (e *ResolveError) Unwrap() error   { return e.cause }
func (e *ResolveError) Code() string    { return e.code }
func (e *ResolveError) Retryable() bool { return e.retryable }
