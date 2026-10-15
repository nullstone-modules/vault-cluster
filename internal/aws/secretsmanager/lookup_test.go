package secretsmanager

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

func TestRegionOf(t *testing.T) {
	got, err := RegionOf("arn:aws:secretsmanager:eu-west-2:123456789012:secret:vault/vault/init/abcDEF-x1Y2z3")
	if err != nil || got != "eu-west-2" {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, bad := range []string{"", "vault/vault/init/abc", "arn:aws:s3:::bucket", "arn:aws:secretsmanager::1:secret:x"} {
		if _, err := RegionOf(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

type fakeLister struct {
	pages [][]types.SecretListEntry
	calls int
}

func (f *fakeLister) ListSecrets(_ context.Context, in *secretsmanager.ListSecretsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error) {
	f.calls++
	i := 0
	if in.NextToken != nil {
		i = int((*in.NextToken)[0] - '0')
	}
	out := &secretsmanager.ListSecretsOutput{SecretList: f.pages[i]}
	if i+1 < len(f.pages) {
		out.NextToken = aws.String(string(rune('0' + i + 1)))
	}
	return out, nil
}

func entry(name, arn string, deleted bool) types.SecretListEntry {
	e := types.SecretListEntry{Name: aws.String(name), ARN: aws.String(arn)}
	if deleted {
		e.DeletedDate = aws.Time(time.Now())
	}
	return e
}

func TestFindByPrefix(t *testing.T) {
	ctx := context.Background()
	one := &fakeLister{pages: [][]types.SecretListEntry{
		{entry("vault/vault/init/old", "arn:old", true)},
		{entry("vault/vault/init/abc", "arn:init", false), entry("vault/vault/initial/abc", "arn:other", false)},
	}}
	got, err := FindByPrefix(ctx, one, "vault/vault/init/")
	if err != nil || got != "arn:init" || one.calls != 2 {
		t.Fatalf("got %q, %v, calls %d", got, err, one.calls)
	}
	none := &fakeLister{pages: [][]types.SecretListEntry{{entry("vault/vault/init/old", "arn:old", true)}}}
	if _, err := FindByPrefix(ctx, none, "vault/vault/init/"); err == nil {
		t.Fatal("deleted-only match accepted")
	}
	two := &fakeLister{pages: [][]types.SecretListEntry{{entry("vault/vault/init/a", "arn:a", false), entry("vault/vault/init/b", "arn:b", false)}}}
	if _, err := FindByPrefix(ctx, two, "vault/vault/init/"); err == nil {
		t.Fatal("ambiguous match accepted")
	}
}
