package secretsmanager

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

// RegionOf returns the region field of a Secrets Manager ARN. Secrets Manager does not redirect a full ARN
// to its region, so every call must be made in that region.
func RegionOf(arn string) (string, error) {
	parts := strings.Split(arn, ":")
	if len(parts) < 6 || parts[0] != "arn" || parts[2] != "secretsmanager" || parts[3] == "" {
		return "", fmt.Errorf("not a Secrets Manager ARN: %q", arn)
	}
	return parts[3], nil
}

// Lister is the subset of the Secrets Manager API that FindByPrefix needs.
type Lister interface {
	ListSecrets(ctx context.Context, params *secretsmanager.ListSecretsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error)
}

// FindByPrefix returns the ARN of the one live secret whose name starts with prefix. It is the fallback for
// clusters applied before the module exported every secret ARN. Zero or several matches are errors.
func FindByPrefix(ctx context.Context, client Lister, prefix string) (string, error) {
	var found []string
	var next *string
	for {
		out, err := client.ListSecrets(ctx, &secretsmanager.ListSecretsInput{
			Filters:   []types.Filter{{Key: types.FilterNameStringTypeName, Values: []string{prefix}}},
			NextToken: next,
		})
		if err != nil {
			return "", fmt.Errorf("list secrets %s*: %w", prefix, err)
		}
		for _, s := range out.SecretList {
			if s.DeletedDate != nil || s.ARN == nil || s.Name == nil || !strings.HasPrefix(*s.Name, prefix) {
				continue
			}
			found = append(found, *s.ARN)
		}
		if out.NextToken == nil {
			break
		}
		next = out.NextToken
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("no secret named %s*", prefix)
	case 1:
		return found[0], nil
	default:
		return "", fmt.Errorf("%d secrets named %s*; pass the ARN explicitly", len(found), prefix)
	}
}

// NewLister returns a Secrets Manager client for one region.
func NewLister(cfg aws.Config, region string) Lister {
	return secretsmanager.NewFromConfig(cfg, func(o *secretsmanager.Options) { o.Region = region })
}
