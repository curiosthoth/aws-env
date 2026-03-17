package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/jmespath/go-jmespath"
)

type secretsManagerAPI interface {
	GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

type ssmAPI interface {
	GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
}

var stringifyJSONResult = stringifyResult
var loadAWSConfig = config.LoadDefaultConfig
var fatalf = log.Fatalf

// CachedSecretsManager resolves and caches values fetched from Secrets Manager and SSM.
type CachedSecretsManager struct {
	rawCache      map[string]string
	jmesPathCache map[string]string

	secretsClient secretsManagerAPI
	ssmClient     ssmAPI
}

// NewCachedSecretsManager creates a resolver backed by the default AWS config chain.
func NewCachedSecretsManager() *CachedSecretsManager {
	loadOptions := []func(*config.LoadOptions) error{config.WithRegion(defaultRegion())}
	if accessKeyID := defaultAccessKeyID(); accessKeyID != "" {
		loadOptions = append(loadOptions, config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, defaultSecretAccessKey(), "")))
	}

	cfg, err := loadAWSConfig(context.TODO(), loadOptions...)
	if err != nil {
		fatalf("unable to load SDK config, %v", err)
	}

	secretsClient := secretsmanager.NewFromConfig(cfg, func(o *secretsmanager.Options) {
		if endpoint := defaultEndpoint(); endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	})
	ssmClient := ssm.NewFromConfig(cfg, func(o *ssm.Options) {
		if endpoint := defaultEndpoint(); endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	})

	return NewCachedSecretsManagerWithClients(secretsClient, ssmClient)
}

// NewCachedSecretsManagerWithClients creates a resolver with injected clients for tests.
func NewCachedSecretsManagerWithClients(secretsClient secretsManagerAPI, ssmClient ssmAPI) *CachedSecretsManager {
	return &CachedSecretsManager{
		secretsClient: secretsClient,
		ssmClient:     ssmClient,
		rawCache:      make(map[string]string),
		jmesPathCache: make(map[string]string),
	}
}

// Get retrieves the remote value and optionally extracts a nested JSON key.
func (v *CachedSecretsManager) Get(scheme string, secretName string, jmesPath *string) (string, bool, string) {
	raw, found, errMsg := v.getRaw(scheme, secretName)
	if !found {
		return "", false, errMsg
	}
	if jmesPath == nil {
		return raw, true, ""
	}
	return v.getByJmesPath(cacheKey(scheme, secretName), raw, *jmesPath)
}

// getRaw retrieves the source value and caches it by backend and key.
func (v *CachedSecretsManager) getRaw(scheme string, secretName string) (string, bool, string) {
	k := cacheKey(scheme, secretName)
	if r, found := v.rawCache[k]; found {
		return r, true, ""
	}

	var (
		r   string
		err error
	)
	switch scheme {
	case SecretsManagerScheme:
		r, err = v.getSecretRaw(secretName)
	case SSMScheme:
		r, err = v.getParameterRaw(secretName)
	default:
		return "", false, fmt.Sprintf("unsupported scheme: %s", scheme)
	}
	if err != nil {
		return "", false, err.Error()
	}

	v.rawCache[k] = r
	return r, true, ""
}

// getSecretRaw loads a secret string from AWS Secrets Manager.
func (v *CachedSecretsManager) getSecretRaw(secretName string) (string, error) {
	result, err := v.secretsClient.GetSecretValue(context.TODO(), &secretsmanager.GetSecretValueInput{
		SecretId: &secretName,
	})
	if err != nil {
		return "", err
	}
	if result.SecretString == nil {
		return "", fmt.Errorf("secret has no string value")
	}
	return *result.SecretString, nil
}

// getParameterRaw loads a decrypted parameter value from AWS SSM Parameter Store.
func (v *CachedSecretsManager) getParameterRaw(parameterName string) (string, error) {
	result, err := v.ssmClient.GetParameter(context.TODO(), &ssm.GetParameterInput{
		Name:           &parameterName,
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return "", err
	}
	if result.Parameter == nil || result.Parameter.Value == nil {
		return "", fmt.Errorf("parameter has no value")
	}
	return *result.Parameter.Value, nil
}

// getByJmesPath extracts and caches a JSON value resolved from a raw source string.
func (v *CachedSecretsManager) getByJmesPath(cachePrefix string, raw string, jmesPath string) (string, bool, string) {
	k := fmt.Sprintf("%s%s%s", cachePrefix, DefaultDelimiter, jmesPath)
	if r, found := v.jmesPathCache[k]; found {
		return r, true, ""
	}

	var data interface{}
	err := json.Unmarshal([]byte(raw), &data)
	if err != nil {
		return "", false, fmt.Sprintf("failed to unmarshal JSON: %v", err)
	}
	r, err := jmespath.Search(jmesPath, data)
	if err != nil {
		return "", false, fmt.Sprintf("JMESPath query failed: %v", err)
	}
	if r == nil {
		return "", false, "JMESPath query returned nil"
	}

	s, err := stringifyJSONResult(r)
	if err != nil {
		return "", false, fmt.Sprintf("failed to format result: %v", err)
	}
	v.jmesPathCache[k] = s
	return s, true, ""
}

func cacheKey(scheme string, name string) string {
	return fmt.Sprintf("%s://%s", scheme, name)
}

func stringifyResult(v interface{}) (string, error) {
	switch typed := v.(type) {
	case string:
		return typed, nil
	case float64:
		if typed == float64(int64(typed)) {
			return fmt.Sprintf("%d", int64(typed)), nil
		}
		return fmt.Sprintf("%f", typed), nil
	case bool:
		return fmt.Sprintf("%t", typed), nil
	default:
		marshal, err := json.Marshal(typed)
		if err != nil {
			return "", fmt.Errorf("failed to marshal result: %v", err)
		}
		return string(marshal), nil
	}
}

func defaultEndpoint() string {
	return os.Getenv("AWS_ENDPOINT_URL")
}

func defaultRegion() string {
	if region := os.Getenv("AWS_REGION"); region != "" {
		return region
	}
	return "us-east-1"
}

func defaultAccessKeyID() string {
	return os.Getenv("AWS_ACCESS_KEY_ID")
}

func defaultSecretAccessKey() string {
	return os.Getenv("AWS_SECRET_ACCESS_KEY")
}
