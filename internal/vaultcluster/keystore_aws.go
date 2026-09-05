package vaultcluster

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/hashicorp/vault/api"
)

type secretKV interface {
	Get(ctx context.Context, arn string) ([]byte, error)
	Put(ctx context.Context, arn string, val []byte) error
}

type SecretsManagerKeyStore struct {
	Secrets         secretKV
	InitARN         string
	ProvisioningARN string
	OperatorARN     string
}

func NewSecretsManagerKeyStore(initARN, provisioningARN, operatorARN string) (*SecretsManagerKeyStore, error) {
	if initARN == "" || provisioningARN == "" || operatorARN == "" {
		return nil, fmt.Errorf("VAULT_INIT_SECRET_ARN, VAULT_PROVISIONING_SECRET_ARN, and VAULT_OPERATOR_SECRET_ARN are required")
	}
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, fmt.Errorf("AWS credentials: %w", err)
	}
	return &SecretsManagerKeyStore{
		Secrets:         smClient{inner: secretsmanager.NewFromConfig(cfg)},
		InitARN:         initARN,
		ProvisioningARN: provisioningARN,
		OperatorARN:     operatorARN,
	}, nil
}

func (s SecretsManagerKeyStore) tokenARN(name string) (string, error) {
	switch name {
	case "provisioning":
		return s.ProvisioningARN, nil
	case "operator":
		return s.OperatorARN, nil
	default:
		return "", fmt.Errorf("unknown token %q", name)
	}
}

func (s SecretsManagerKeyStore) SaveInit(resp *api.InitResponse) error {
	b, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return s.Secrets.Put(context.Background(), s.InitARN, b)
}

func (s SecretsManagerKeyStore) LoadInit() (*api.InitResponse, error) {
	raw, err := s.Secrets.Get(context.Background(), s.InitARN)
	if err != nil {
		return nil, err
	}
	var resp api.InitResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (s SecretsManagerKeyStore) SaveToken(name, token string) error {
	arn, err := s.tokenARN(name)
	if err != nil {
		return err
	}
	return s.Secrets.Put(context.Background(), arn, []byte(token))
}

func (s SecretsManagerKeyStore) LoadToken(name string) (string, error) {
	arn, err := s.tokenARN(name)
	if err != nil {
		return "", err
	}
	b, err := s.Secrets.Get(context.Background(), arn)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

type smClient struct {
	inner *secretsmanager.Client
}

func (c smClient) Get(ctx context.Context, arn string) ([]byte, error) {
	out, err := c.inner.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(arn),
	})
	if err != nil {
		return nil, fmt.Errorf("secrets manager get %s: %w", arn, err)
	}
	if out.SecretString != nil {
		return []byte(*out.SecretString), nil
	}
	if len(out.SecretBinary) > 0 {
		return out.SecretBinary, nil
	}
	return nil, fmt.Errorf("secrets manager get %s: empty secret", arn)
}

func (c smClient) Put(ctx context.Context, arn string, val []byte) error {
	_, err := c.inner.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
		SecretId:     aws.String(arn),
		SecretString: aws.String(string(val)),
	})
	if err != nil {
		return fmt.Errorf("secrets manager put %s: %w", arn, err)
	}
	return nil
}
