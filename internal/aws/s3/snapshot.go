package s3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

type ObjectStore interface {
	Put(ctx context.Context, bucket, key string, body []byte) error
	List(ctx context.Context, bucket, prefix string) ([]string, error)
}

type Store struct {
	inner *awss3.Client
}

func New() (Store, error) {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return Store{}, fmt.Errorf("AWS credentials: %w", err)
	}
	return Store{inner: awss3.NewFromConfig(cfg)}, nil
}

func (s Store) Put(ctx context.Context, bucket, key string, body []byte) error {
	_, err := s.inner.PutObject(ctx, &awss3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(body),
	})
	if err != nil {
		return fmt.Errorf("s3 put s3://%s/%s: %w", bucket, key, err)
	}
	return nil
}

func (s Store) List(ctx context.Context, bucket, prefix string) ([]string, error) {
	out, err := s.inner.ListObjectsV2(ctx, &awss3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 list s3://%s/%s: %w", bucket, prefix, err)
	}
	var keys []string
	for _, obj := range out.Contents {
		if obj.Key == nil || !strings.HasSuffix(*obj.Key, ".snap") {
			continue
		}
		keys = append(keys, "s3://"+bucket+"/"+*obj.Key)
	}
	return keys, nil
}

func normalizePrefix(prefix string) string {
	if p := strings.Trim(prefix, "/"); p != "" {
		return p
	}
	return "vault-snapshots"
}

func ObjectKey(prefix, stamp string) string {
	return normalizePrefix(prefix) + "/vault-" + stamp + ".snap"
}

func PutSnapshot(store ObjectStore, bucket, prefix string, data []byte) (string, error) {
	if bucket == "" {
		return "", fmt.Errorf("SNAPSHOT_BUCKET is not set")
	}
	if len(data) == 0 {
		return "", fmt.Errorf("snapshot is empty; refusing to keep it")
	}
	key := ObjectKey(prefix, time.Now().UTC().Format("20060102T150405Z"))
	if err := store.Put(context.Background(), bucket, key, data); err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	if err := store.Put(context.Background(), bucket, key+".sha256", []byte(hex.EncodeToString(sum[:])+"\n")); err != nil {
		return "", err
	}
	return "s3://" + bucket + "/" + key, nil
}

func ListSnapshots(store ObjectStore, bucket, prefix string) ([]string, error) {
	if bucket == "" {
		return nil, fmt.Errorf("SNAPSHOT_BUCKET is not set")
	}
	return store.List(context.Background(), bucket, normalizePrefix(prefix)+"/")
}
