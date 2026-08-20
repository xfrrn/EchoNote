package podcast

import (
	"context"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"

	domain "github.com/Actify/echonote/apps/server/internal/domain/podcast"
)

type directAudioResolver struct {
	client *http.Client
}

func newDirectAudioResolver(client *http.Client) *directAudioResolver {
	return &directAudioResolver{client: client}
}

func (r *directAudioResolver) CanResolve(rawURL string) bool {
	_, err := parsePublicURL(rawURL)
	return err == nil
}

func (r *directAudioResolver) Resolve(ctx context.Context, rawURL string) (*domain.ResolvedEpisode, error) {
	parsed, err := parsePublicURL(rawURL)
	if err != nil {
		return nil, domain.NewResolveError("IMPORT_INVALID_URL", "URL is invalid", false, err)
	}

	response, err := r.probe(ctx, parsed.String(), http.MethodHead)
	if err != nil {
		if response != nil {
			response.Body.Close()
		}
		response, err = r.probe(ctx, parsed.String(), http.MethodGet)
		if err != nil {
			return nil, domain.NewResolveError("IMPORT_SOURCE_UNAVAILABLE", "Audio source is unavailable", true, err)
		}
	}
	if response.StatusCode == http.StatusBadRequest || response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusMethodNotAllowed || response.StatusCode == http.StatusNotImplemented {
		response.Body.Close()
		response, err = r.probe(ctx, parsed.String(), http.MethodGet)
		if err != nil {
			return nil, domain.NewResolveError("IMPORT_SOURCE_UNAVAILABLE", "Audio source is unavailable", true, err)
		}
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if hasAudioExtension(parsed.Path) {
			retryable := response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
			return nil, domain.NewResolveError("IMPORT_AUDIO_FETCH_FAILED", "Audio source could not be fetched", retryable, nil)
		}
		return nil, domain.ErrNotApplicable
	}
	if !isAudioResponse(response.Header.Get("Content-Type"), parsed.Path) {
		return nil, domain.ErrNotApplicable
	}

	audioURL := response.Request.URL.String()
	if _, err := parsePublicURL(audioURL); err != nil {
		return nil, domain.NewResolveError("IMPORT_AUDIO_URL_INVALID", "Audio source returned an invalid URL", false, err)
	}
	return &domain.ResolvedEpisode{
		SourceType:   domain.SourceDirectAudio,
		EpisodeTitle: audioTitle(parsed, response.Header.Get("Content-Disposition")),
		CanonicalURL: parsed.String(),
		AudioURL:     audioURL,
	}, nil
}

func (r *directAudioResolver) probe(ctx context.Context, rawURL, method string) (*http.Response, error) {
	request, err := newRequest(ctx, method, rawURL)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "audio/*")
	if method == http.MethodGet {
		request.Header.Set("Range", "bytes=0-0")
	}
	return r.client.Do(request)
}

func isAudioResponse(contentType, rawPath string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.TrimSpace(strings.Split(contentType, ";")[0])
	}
	return strings.HasPrefix(strings.ToLower(mediaType), "audio/") ||
		(hasAudioExtension(rawPath) && (mediaType == "" || strings.EqualFold(mediaType, "application/octet-stream")))
}

func hasAudioExtension(rawPath string) bool {
	switch strings.ToLower(path.Ext(rawPath)) {
	case ".mp3", ".m4a", ".aac", ".wav", ".flac", ".ogg", ".opus":
		return true
	default:
		return false
	}
}

func audioTitle(parsed *url.URL, contentDisposition string) string {
	filename := ""
	if _, parameters, err := mime.ParseMediaType(contentDisposition); err == nil {
		filename = parameters["filename"]
	}
	if filename == "" {
		filename = path.Base(parsed.Path)
	}
	if unescaped, err := url.PathUnescape(filename); err == nil {
		filename = unescaped
	}
	title := strings.TrimSpace(strings.TrimSuffix(filename, path.Ext(filename)))
	if title == "" || title == "." {
		return "Imported audio"
	}
	return title
}
