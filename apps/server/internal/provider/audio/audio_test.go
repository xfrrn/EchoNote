package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestDownloaderLimitsContentTypeAndHashes(t *testing.T) {
	downloader := &Downloader{client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Referer") != "https://social.example.com/" || request.Header["User-Agent"][0] != "" {
			t.Fatalf("unexpected download headers: %v", request.Header)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"audio/mpeg"}},
			Body:       io.NopCloser(bytes.NewReader([]byte("audio"))),
		}, nil
	})}}
	destination := filepath.Join(t.TempDir(), "audio.bin")
	hash, err := downloader.Download(context.Background(), "https://cdn.example.com/audio.mp3", map[string]string{
		"Referer": "https://social.example.com/", "User-Agent": "",
	}, destination)
	if err != nil || hash != "6ed8919ce20490a5e3ad8630a4fab69475297abd07db73918dd5f36fcfaeb11b" {
		t.Fatalf("hash=%q err=%v", hash, err)
	}
}

func TestFFmpegPrepareAndRender(t *testing.T) {
	processor, err := NewFFmpeg("ffmpeg", "ffprobe")
	if err != nil {
		t.Skip(err)
	}
	directory := t.TempDir()
	input := filepath.Join(directory, "input.wav")
	writeSilentWAV(t, input, 2)
	prepared := filepath.Join(directory, "prepared.flac")
	duration, preparedHash, err := processor.Prepare(context.Background(), input, prepared)
	if err != nil || duration < 1900 || duration > 2100 || len(preparedHash) != 64 {
		t.Fatalf("duration=%d hash=%q err=%v", duration, preparedHash, err)
	}
	chunk := filepath.Join(directory, "chunk.flac")
	chunkHash, err := processor.Render(context.Background(), prepared, 500, 1500, chunk)
	if err != nil || len(chunkHash) != 64 {
		t.Fatalf("chunk hash=%q err=%v", chunkHash, err)
	}
}

func writeSilentWAV(t *testing.T, filePath string, seconds int) {
	t.Helper()
	const sampleRate = 16000
	dataSize := sampleRate * seconds * 2
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	write := func(value any) {
		if err := binary.Write(file, binary.LittleEndian, value); err != nil {
			t.Fatal(err)
		}
	}
	_, _ = file.WriteString("RIFF")
	write(uint32(36 + dataSize))
	_, _ = file.WriteString("WAVEfmt ")
	write(uint32(16))
	write(uint16(1))
	write(uint16(1))
	write(uint32(sampleRate))
	write(uint32(sampleRate * 2))
	write(uint16(2))
	write(uint16(16))
	_, _ = file.WriteString("data")
	write(uint32(dataSize))
	if _, err := io.CopyN(file, bytes.NewReader(make([]byte, dataSize)), int64(dataSize)); err != nil {
		t.Fatal(err)
	}
}
