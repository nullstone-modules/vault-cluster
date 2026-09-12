package s3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

type ObjectStore interface {
	Put(ctx context.Context, bucket, key string, body []byte) error
	PutIfAbsent(ctx context.Context, bucket, key string, body []byte) (bool, error)
	Get(ctx context.Context, bucket, key string) ([]byte, error)
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

func (s Store) PutIfAbsent(ctx context.Context, bucket, key string, body []byte) (bool, error) {
	_, err := s.inner.PutObject(ctx, &awss3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(body),
		IfNoneMatch: aws.String("*"),
	})
	if err != nil {
		if isPreconditionFailed(err) {
			return false, nil
		}
		return false, fmt.Errorf("s3 put-if-absent s3://%s/%s: %w", bucket, key, err)
	}
	return true, nil
}

func (s Store) Get(ctx context.Context, bucket, key string) ([]byte, error) {
	out, err := s.inner.GetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 get s3://%s/%s: %w", bucket, key, err)
	}
	defer out.Body.Close()
	b, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, fmt.Errorf("s3 get s3://%s/%s: %w", bucket, key, err)
	}
	return b, nil
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

func isPreconditionFailed(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) && apiErr.ErrorCode() == "PreconditionFailed" {
		return true
	}
	return strings.Contains(err.Error(), "PreconditionFailed")
}

func ClaimInit(store ObjectStore, bucket, prefix, nodeID string) (bool, error) {
	if bucket == "" {
		return false, fmt.Errorf("SNAPSHOT_BUCKET is not set")
	}
	key := normalizePrefix(prefix) + "/.init-claim"
	won, err := store.PutIfAbsent(context.Background(), bucket, key, []byte(nodeID))
	if err != nil || won {
		return won, err
	}
	body, err := store.Get(context.Background(), bucket, key)
	if err != nil {
		return false, err
	}
	holder := strings.TrimSpace(string(body))
	if holder == "" || holder == nodeID {
		if err := store.Put(context.Background(), bucket, key, []byte(nodeID)); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
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
	keys, err := store.List(context.Background(), bucket, normalizePrefix(prefix)+"/")
	if err != nil {
		return nil, err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(keys)))
	return keys, nil
}

func ParseObjectURI(uri string) (bucket, key string, err error) {
	rest, ok := strings.CutPrefix(uri, "s3://")
	if !ok {
		return "", "", fmt.Errorf("not an s3 uri: %s", uri)
	}
	bucket, key, ok = strings.Cut(rest, "/")
	if !ok || bucket == "" || key == "" || !strings.HasSuffix(key, ".snap") {
		return "", "", fmt.Errorf("invalid snapshot uri %q", uri)
	}
	return bucket, key, nil
}

func GetSnapshot(store ObjectStore, uri string) ([]byte, error) {
	bucket, key, err := ParseObjectURI(uri)
	if err != nil {
		return nil, err
	}
	data, err := store.Get(context.Background(), bucket, key)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("snapshot is empty; refusing to restore %s", uri)
	}
	want, err := store.Get(context.Background(), bucket, key+".sha256")
	if err != nil {
		return nil, fmt.Errorf("no checksum beside %s; integrity cannot be established", uri)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != strings.TrimSpace(string(want)) {
		return nil, fmt.Errorf("checksum mismatch for %s; this snapshot is corrupt and must not be restored", uri)
	}
	return data, nil
}
