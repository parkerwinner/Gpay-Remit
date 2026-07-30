package secrets

import (
	"context"
	"fmt"
	"sync"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// secretsManagerClient is the subset of *secretsmanager.Client this package
// depends on. Defined as an interface (rather than depending on the
// concrete client directly) so tests can inject a fake instead of making
// real AWS calls.
type secretsManagerClient interface {
	GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

type cacheEntry struct {
	value     string
	expiresAt time.Time
}

// AWSSecretsManagerProvider resolves secrets from AWS Secrets Manager,
// caching each value for cacheTTL to avoid an API call (and its associated
// latency/cost) on every single config read.
type AWSSecretsManagerProvider struct {
	client   secretsManagerClient
	cacheTTL time.Duration

	mu    sync.RWMutex
	cache map[string]cacheEntry
}

const defaultCacheTTL = 5 * time.Minute

// NewAWSSecretsManagerProvider builds a provider using the standard AWS SDK
// credential chain (environment variables, shared config/credentials
// files, EC2/ECS/Lambda instance role, etc.) — no AWS credentials are
// hardcoded or required directly by this package.
func NewAWSSecretsManagerProvider(ctx context.Context) (*AWSSecretsManagerProvider, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config for Secrets Manager: %w", err)
	}

	return newAWSSecretsManagerProviderWithClient(secretsmanager.NewFromConfig(cfg), defaultCacheTTL), nil
}

// newAWSSecretsManagerProviderWithClient is the test seam: production code
// always goes through NewAWSSecretsManagerProvider, tests construct this
// directly with a fake client.
func newAWSSecretsManagerProviderWithClient(client secretsManagerClient, cacheTTL time.Duration) *AWSSecretsManagerProvider {
	return &AWSSecretsManagerProvider{
		client:   client,
		cacheTTL: cacheTTL,
		cache:    make(map[string]cacheEntry),
	}
}

// GetSecret returns the current value of the named secret (by name or
// ARN), using a cached value if one was fetched within cacheTTL.
func (p *AWSSecretsManagerProvider) GetSecret(ctx context.Context, name string) (string, error) {
	if cached, ok := p.getCached(name); ok {
		return cached, nil
	}

	output, err := p.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: &name,
	})
	if err != nil {
		return "", fmt.Errorf("fetching secret %q from AWS Secrets Manager: %w", name, err)
	}

	if output.SecretString == nil {
		return "", fmt.Errorf("secret %q has no SecretString value (binary secrets are not supported)", name)
	}

	value := *output.SecretString
	p.setCached(name, value)
	return value, nil
}

func (p *AWSSecretsManagerProvider) getCached(name string) (string, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	entry, ok := p.cache[name]
	if !ok || time.Now().After(entry.expiresAt) {
		return "", false
	}
	return entry.value, true
}

func (p *AWSSecretsManagerProvider) setCached(name, value string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.cache[name] = cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(p.cacheTTL),
	}
}
