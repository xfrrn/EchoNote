package audio

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Actify/echonote/apps/server/internal/provider/safehttp"
)

const maxAudioBytes int64 = 2 << 30

type ProviderError struct {
	code      string
	message   string
	retryable bool
}

func (err *ProviderError) Error() string   { return err.message }
func (err *ProviderError) Code() string    { return err.code }
func (err *ProviderError) Retryable() bool { return err.retryable }

type Downloader struct {
	client *http.Client
}

func NewDownloader() *Downloader {
	return &Downloader{client: safehttp.NewClient(30 * time.Minute)}
}

func (downloader *Downloader) Download(ctx context.Context, rawURL string, headers map[string]string, destination string) (string, error) {
	parsed, err := safehttp.ParsePublicURL(rawURL)
	if err != nil {
		return "", &ProviderError{code: "AUDIO_URL_INVALID", message: err.Error()}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", err
	}
	hasUserAgent := false
	for name, value := range headers {
		request.Header.Set(name, value)
		hasUserAgent = hasUserAgent || strings.EqualFold(name, "User-Agent")
	}
	if !hasUserAgent {
		request.Header.Set("User-Agent", "EchoNote/0.5")
	}
	response, err := downloader.client.Do(request)
	if err != nil {
		return "", &ProviderError{code: "AUDIO_DOWNLOAD_FAILED", message: err.Error(), retryable: true}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", &ProviderError{
			code: "AUDIO_DOWNLOAD_HTTP_ERROR", message: fmt.Sprintf("audio download returned HTTP %d", response.StatusCode),
			retryable: response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500,
		}
	}
	if response.ContentLength > maxAudioBytes {
		return "", &ProviderError{code: "AUDIO_TOO_LARGE", message: "audio exceeds 2 GiB limit"}
	}
	if !supportedContentType(response.Header.Get("Content-Type"), parsed.Path) {
		return "", &ProviderError{code: "AUDIO_CONTENT_TYPE_INVALID", message: "response is not a supported audio container"}
	}
	file, err := os.Create(destination)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, maxAudioBytes+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		return "", errors.Join(copyErr, closeErr)
	}
	if written > maxAudioBytes {
		return "", &ProviderError{code: "AUDIO_TOO_LARGE", message: "audio exceeds 2 GiB limit"}
	}
	if written == 0 {
		return "", &ProviderError{code: "AUDIO_EMPTY", message: "audio response was empty"}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func supportedContentType(header, filePath string) bool {
	contentType, _, _ := mime.ParseMediaType(header)
	if strings.HasPrefix(contentType, "audio/") || strings.HasPrefix(contentType, "video/") || contentType == "application/octet-stream" {
		return true
	}
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".aac", ".amr", ".avi", ".flac", ".flv", ".m4a", ".mkv", ".mov", ".mp3", ".mp4", ".mpeg", ".ogg", ".opus", ".wav", ".webm", ".wma", ".wmv":
		return contentType == ""
	default:
		return false
	}
}

type FFmpeg struct {
	ffmpeg, ffprobe string
}

func NewFFmpeg(ffmpegPath, ffprobePath string) (*FFmpeg, error) {
	ffmpeg, err := exec.LookPath(strings.TrimSpace(ffmpegPath))
	if err != nil {
		return nil, fmt.Errorf("find ffmpeg: %w", err)
	}
	ffprobe, err := exec.LookPath(strings.TrimSpace(ffprobePath))
	if err != nil {
		return nil, fmt.Errorf("find ffprobe: %w", err)
	}
	return &FFmpeg{ffmpeg: ffmpeg, ffprobe: ffprobe}, nil
}

func (processor *FFmpeg) Prepare(ctx context.Context, source, destination string) (int64, string, error) {
	if err := processor.run(ctx, source,
		"-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-i", source,
		"-map", "0:a:0", "-vn", "-ac", "1", "-ar", "16000", "-c:a", "flac", destination,
	); err != nil {
		return 0, "", err
	}
	duration, err := processor.probe(ctx, destination)
	if err != nil {
		return 0, "", err
	}
	hash, err := hashFile(destination)
	return duration, hash, err
}

func (processor *FFmpeg) Render(ctx context.Context, source string, startMS, endMS int64, destination string) (string, error) {
	if startMS < 0 || endMS <= startMS {
		return "", errors.New("invalid chunk interval")
	}
	if err := processor.run(ctx, source,
		"-nostdin", "-hide_banner", "-loglevel", "error", "-y",
		"-ss", milliseconds(startMS), "-i", source, "-t", milliseconds(endMS-startMS),
		"-map", "0:a:0", "-vn", "-ac", "1", "-ar", "16000", "-c:a", "flac", destination,
	); err != nil {
		return "", err
	}
	return hashFile(destination)
}

func (processor *FFmpeg) run(ctx context.Context, source string, arguments ...string) error {
	output, err := exec.CommandContext(ctx, processor.ffmpeg, arguments...).CombinedOutput()
	if err == nil {
		return nil
	}
	message := strings.ReplaceAll(string(output), source, "[audio-source]")
	if len(message) > 4096 {
		message = message[len(message)-4096:]
	}
	return &ProviderError{code: "AUDIO_PROCESSING_FAILED", message: "ffmpeg: " + strings.TrimSpace(message), retryable: true}
}

func (processor *FFmpeg) probe(ctx context.Context, filePath string) (int64, error) {
	output, err := exec.CommandContext(ctx, processor.ffprobe,
		"-v", "error", "-show_entries", "format=duration", "-of", "json", filePath,
	).Output()
	if err != nil {
		return 0, &ProviderError{code: "AUDIO_PROBE_FAILED", message: "ffprobe failed", retryable: true}
	}
	var result struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return 0, fmt.Errorf("decode ffprobe output: %w", err)
	}
	seconds, err := strconv.ParseFloat(result.Format.Duration, 64)
	if err != nil || seconds <= 0 {
		return 0, errors.New("ffprobe returned invalid duration")
	}
	return int64(seconds*1000 + 0.5), nil
}

func hashFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func milliseconds(value int64) string {
	return strconv.FormatFloat(float64(value)/1000, 'f', 3, 64)
}
