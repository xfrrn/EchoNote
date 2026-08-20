package storage

import (
	"context"
	"net/url"
	"testing"
	"time"
)

func TestValidateObjectKey(t *testing.T) {
	for _, key := range []string{"", "/root", "../secret", "users/x/../secret", `users\x`} {
		if validateKey(key) == nil {
			t.Fatalf("key %q should be rejected", key)
		}
	}
	if err := validateKey("users/u/episodes/e/transcription-runs/r/chunks/000.flac"); err != nil {
		t.Fatal(err)
	}
}

func TestAliyunOSSPresignsWithoutNetwork(t *testing.T) {
	store, err := NewAliyunOSS(AliyunOSSConfig{
		Region: "cn-beijing", Endpoint: "https://oss-cn-beijing.aliyuncs.com", Bucket: "echonote-test",
		AccessKey: "test-id", SecretKey: "test-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	signed, err := store.SignedURL(context.Background(), "users/u/chunk.flac", time.Minute)
	parsed, parseErr := url.Parse(signed)
	if err != nil || parseErr != nil || parsed.Scheme != "https" || parsed.Query().Get("x-oss-signature") == "" {
		t.Fatalf("signed=%q err=%v parseErr=%v", signed, err, parseErr)
	}
	if _, err := NewAliyunOSS(AliyunOSSConfig{
		Region: "cn-beijing", Endpoint: "http://user:secret@localhost", Bucket: "echonote-test",
		AccessKey: "test-id", SecretKey: "test-secret",
	}); err == nil {
		t.Fatal("expected insecure endpoint to fail")
	}
}
