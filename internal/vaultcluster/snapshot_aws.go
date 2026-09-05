package vaultcluster

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type objectStore interface {
	Put(ctx context.Context, bucket, key string, body []byte) error
	List(ctx context.Context, bucket, prefix string) ([]string, error)
}

type s3Store struct {
	inner *s3.Client
}

func NewS3ObjectStore() (objectStore, error) {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, fmt.Errorf("AWS credentials: %w", err)
	}
	return s3Store{inner: s3.NewFromConfig(cfg)}, nil
}

func (s s3Store) Put(ctx context.Context, bucket, key string, body []byte) error {
	_, err := s.inner.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(body),
	})
	if err != nil {
		return fmt.Errorf("s3 put s3://%s/%s: %w", bucket, key, err)
	}
	return nil
}

func (s s3Store) List(ctx context.Context, bucket, prefix string) ([]string, error) {
	out, err := s.inner.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
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

func snapshotObjectKey(prefix, stamp string) string {
	p := strings.Trim(prefix, "/")
	if p == "" {
		p = "vault-snapshots"
	}
	return p + "/vault-" + stamp + ".snap"
}

func (c *Client) SnapshotTakeS3(store objectStore, bucket, prefix string) (string, error) {
	if bucket == "" {
		return "", fmt.Errorf("SNAPSHOT_BUCKET is not set")
	}
	b, err := c.raftSnapshot()
	if err != nil {
		return "", err
	}
	key := snapshotObjectKey(prefix, snapshotStamp())
	if err := store.Put(context.Background(), bucket, key, b); err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	if err := store.Put(context.Background(), bucket, key+".sha256", []byte(hex.EncodeToString(sum[:])+"\n")); err != nil {
		return "", err
	}
	return "s3://" + bucket + "/" + key, nil
}

func SnapshotListS3(store objectStore, bucket, prefix string) ([]string, error) {
	if bucket == "" {
		return nil, fmt.Errorf("SNAPSHOT_BUCKET is not set")
	}
	p := strings.Trim(prefix, "/")
	if p == "" {
		p = "vault-snapshots"
	}
	return store.List(context.Background(), bucket, p+"/")
}
