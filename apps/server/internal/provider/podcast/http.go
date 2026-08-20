package podcast

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/Actify/echonote/apps/server/internal/provider/safehttp"
)

const maxMetadataBytes = 5 << 20

func NewHTTPClient() *http.Client {
	return safehttp.NewClient(20 * time.Second)
}

func parsePublicURL(rawURL string) (*url.URL, error) {
	return safehttp.ParsePublicURL(rawURL)
}

func validateURL(parsed *url.URL) error {
	return safehttp.ValidateURL(parsed)
}

func readLimited(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxMetadataBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxMetadataBytes {
		return nil, errors.New("metadata response exceeds 5 MiB")
	}
	return data, nil
}

func newRequest(ctx context.Context, method, rawURL string) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "EchoNote/0.2")
	return request, nil
}
