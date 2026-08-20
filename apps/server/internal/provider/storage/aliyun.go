package storage

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/url"
	"path"
	"strings"
	"time"

	domain "github.com/Actify/echonote/apps/server/internal/domain/transcription"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"
)

var _ domain.ObjectStore = (*AliyunOSS)(nil)

type AliyunOSS struct {
	client *oss.Client
	bucket string
}

type AliyunOSSConfig struct {
	Region, Endpoint, Bucket, AccessKey, SecretKey string
}

func NewAliyunOSS(config AliyunOSSConfig) (*AliyunOSS, error) {
	if strings.TrimSpace(config.Region) == "" || strings.TrimSpace(config.Bucket) == "" ||
		strings.TrimSpace(config.AccessKey) == "" || strings.TrimSpace(config.SecretKey) == "" {
		return nil, errors.New("OSS region, bucket, access key, and secret key are required")
	}
	if endpoint := strings.TrimSpace(config.Endpoint); endpoint != "" {
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
			return nil, errors.New("OSS endpoint must be an HTTPS URL without credentials")
		}
	}
	sdkConfig := oss.LoadDefaultConfig().
		WithRegion(strings.TrimSpace(config.Region)).
		WithCredentialsProvider(credentials.NewStaticCredentialsProvider(strings.TrimSpace(config.AccessKey), strings.TrimSpace(config.SecretKey))).
		WithRetryMaxAttempts(3)
	if endpoint := strings.TrimSpace(config.Endpoint); endpoint != "" {
		sdkConfig.WithEndpoint(endpoint)
	}
	return &AliyunOSS{client: oss.NewClient(sdkConfig), bucket: strings.TrimSpace(config.Bucket)}, nil
}

func (store *AliyunOSS) Put(ctx context.Context, key string, reader io.Reader) error {
	if err := validateKey(key); err != nil {
		return err
	}
	contentType := mime.TypeByExtension(path.Ext(key))
	request := &oss.PutObjectRequest{Bucket: oss.Ptr(store.bucket), Key: oss.Ptr(key), Body: reader}
	if contentType != "" {
		request.ContentType = oss.Ptr(contentType)
	}
	_, err := store.client.PutObject(ctx, request)
	return err
}

func (store *AliyunOSS) Delete(ctx context.Context, key string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	_, err := store.client.DeleteObject(ctx, &oss.DeleteObjectRequest{Bucket: oss.Ptr(store.bucket), Key: oss.Ptr(key)})
	return err
}

func (store *AliyunOSS) SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if err := validateKey(key); err != nil {
		return "", err
	}
	if ttl <= 0 || ttl > 7*24*time.Hour {
		return "", errors.New("signed URL TTL must be between 1ns and 7 days")
	}
	result, err := store.client.Presign(ctx, &oss.GetObjectRequest{Bucket: oss.Ptr(store.bucket), Key: oss.Ptr(key)}, oss.PresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return result.URL, nil
}

func validateKey(key string) error {
	key = strings.TrimSpace(key)
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, `\`) || path.Clean(key) != key || strings.HasPrefix(key, "../") {
		return errors.New("invalid object key")
	}
	return nil
}
