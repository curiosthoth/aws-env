package internal

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSecretsManagerClient struct {
	values map[string]*string
	err    error
	calls  int
}

func (f *fakeSecretsManagerClient) GetSecretValue(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if v, ok := f.values[*params.SecretId]; ok {
		return &secretsmanager.GetSecretValueOutput{SecretString: v}, nil
	}
	return nil, errors.New("secret not found")
}

type fakeSSMClient struct {
	values map[string]*string
	err    error
	calls  int
}

func (f *fakeSSMClient) GetParameter(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if v, ok := f.values[*params.Name]; ok {
		return &ssm.GetParameterOutput{Parameter: &ssmtypes.Parameter{Value: v}}, nil
	}
	return nil, errors.New("parameter not found")
}

func ptr[T any](v T) *T {
	return &v
}

func TestCachedSecretsManagerGetByJmesPath(t *testing.T) {
	m := NewCachedSecretsManagerWithClients(&fakeSecretsManagerClient{}, &fakeSSMClient{})

	s, found, _ := m.getByJmesPath("secretName0", `{"a": {"b": {"c": "value"}}}`, "a.b.c")
	assert.Equal(t, "value", s)
	assert.True(t, found)

	s, found, _ = m.getByJmesPath("secretName1", `{"a": {"b": {"c": "value"}}}`, "a")
	assert.Equal(t, `{"b":{"c":"value"}}`, s)
	assert.True(t, found)

	s, found, _ = m.getByJmesPath("secretName2", `{"a": {"b": {"c": 132}}}`, "a.b.c")
	assert.Equal(t, "132", s)
	assert.True(t, found)

	s, found, errMsg := m.getByJmesPath("secretName3", `{"a": {"b": {"c": "value"}}}`, "m")
	assert.Equal(t, "", s)
	assert.False(t, found)
	assert.NotEmpty(t, errMsg)

	s, found, _ = m.getByJmesPath("secretName4", `{"a": {"b": {"c": 98.1}}}`, "a.b.c")
	s2, _ := strconv.ParseFloat(s, 64)
	assert.Equal(t, 98.1, s2)
	assert.True(t, found)

	s, found, _ = m.getByJmesPath("secretName5", `{"a": {"b": {"c": true}}}`, "a.b.c")
	assert.Equal(t, "true", s)
	assert.True(t, found)

	s, found, errMsg = m.getByJmesPath("secretName6", `{"a": 1}`, "a[")
	assert.Empty(t, s)
	assert.False(t, found)
	assert.Contains(t, errMsg, "JMESPath query failed")

	originalStringify := stringifyJSONResult
	stringifyJSONResult = func(interface{}) (string, error) {
		return "", errors.New("format error")
	}
	defer func() {
		stringifyJSONResult = originalStringify
	}()

	s, found, errMsg = m.getByJmesPath("secretName7", `{"a": {"b": "value"}}`, "a")
	assert.Empty(t, s)
	assert.False(t, found)
	assert.Contains(t, errMsg, "failed to format result")
}

func TestCachedSecretsManagerGetByJmesPathCaching(t *testing.T) {
	m := NewCachedSecretsManagerWithClients(&fakeSecretsManagerClient{}, &fakeSSMClient{})

	s, found, _ := m.getByJmesPath("secretName", `{"a": {"b": {"c": "value"}}}`, "a.b.c")
	assert.Equal(t, "value", s)
	assert.True(t, found)

	s, found, _ = m.getByJmesPath("secretName", `{"a": {"b": {"c": "newvalue"}}}`, "a.b.c")
	assert.Equal(t, "value", s)
	assert.True(t, found)
}

func TestCachedSecretsManagerGetRawAndCaching(t *testing.T) {
	secretsClient := &fakeSecretsManagerClient{values: map[string]*string{
		"secret/name": ptr(`{"foo":{"bar":"baz"}}`),
	}}
	ssmClient := &fakeSSMClient{values: map[string]*string{
		"/app/config": ptr(`{"foo":{"bar":"qux"}}`),
	}}
	m := NewCachedSecretsManagerWithClients(secretsClient, ssmClient)

	value, found, errMsg := m.Get(SecretsManagerScheme, "secret/name", ptr("foo.bar"))
	require.True(t, found)
	assert.Equal(t, "baz", value)
	assert.Empty(t, errMsg)

	value, found, errMsg = m.Get(SSMScheme, "/app/config", ptr("foo.bar"))
	require.True(t, found)
	assert.Equal(t, "qux", value)
	assert.Empty(t, errMsg)

	_, _, _ = m.Get(SecretsManagerScheme, "secret/name", nil)
	_, _, _ = m.Get(SSMScheme, "/app/config", nil)
	assert.Equal(t, 1, secretsClient.calls)
	assert.Equal(t, 1, ssmClient.calls)
}

func TestCachedSecretsManagerErrors(t *testing.T) {
	secretsClient := &fakeSecretsManagerClient{err: errors.New("boom")}
	ssmClient := &fakeSSMClient{err: errors.New("bang")}
	m := NewCachedSecretsManagerWithClients(secretsClient, ssmClient)

	_, found, errMsg := m.Get(SecretsManagerScheme, "secret/name", nil)
	assert.False(t, found)
	assert.Contains(t, errMsg, "boom")

	_, found, errMsg = m.Get(SSMScheme, "/param", nil)
	assert.False(t, found)
	assert.Contains(t, errMsg, "bang")

	_, found, errMsg = m.Get("unknown", "value", nil)
	assert.False(t, found)
	assert.Contains(t, errMsg, "unsupported scheme")

	_, found, errMsg = m.getByJmesPath("cache", "not-json", "foo")
	assert.False(t, found)
	assert.Contains(t, errMsg, "failed to unmarshal JSON")

	nilSecretClient := &fakeSecretsManagerClient{values: map[string]*string{"empty": nil}}
	nilParameterClient := &fakeSSMClient{values: map[string]*string{"empty": nil}}
	nilValueManager := NewCachedSecretsManagerWithClients(nilSecretClient, nilParameterClient)

	_, found, errMsg = nilValueManager.Get(SecretsManagerScheme, "empty", nil)
	assert.False(t, found)
	assert.Contains(t, errMsg, "secret has no string value")

	_, found, errMsg = nilValueManager.Get(SSMScheme, "empty", nil)
	assert.False(t, found)
	assert.Contains(t, errMsg, "parameter has no value")
}

func TestHelpers(t *testing.T) {
	t.Setenv("AWS_ENDPOINT_URL", "http://localhost:5000")
	t.Setenv("AWS_REGION", "us-west-2")
	t.Setenv("AWS_ACCESS_KEY_ID", "key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")

	assert.Equal(t, "http://localhost:5000", defaultEndpoint())
	assert.Equal(t, "us-west-2", defaultRegion())
	assert.Equal(t, "key", defaultAccessKeyID())
	assert.Equal(t, "secret", defaultSecretAccessKey())
	assert.Equal(t, "secretsmanager://name", cacheKey(SecretsManagerScheme, "name"))

	s, err := stringifyResult(map[string]string{"a": "b"})
	require.NoError(t, err)
	assert.Equal(t, `{"a":"b"}`, s)

	s, err = stringifyResult(make(chan int))
	require.Error(t, err)
	assert.Empty(t, s)
}

func TestDefaultsAndConstructor(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		t.Setenv("AWS_ENDPOINT_URL", "")
		t.Setenv("AWS_REGION", "")
		t.Setenv("AWS_ACCESS_KEY_ID", "")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "")

		assert.Equal(t, "", defaultEndpoint())
		assert.Equal(t, "us-east-1", defaultRegion())

		manager := NewCachedSecretsManager()
		require.NotNil(t, manager)
		require.NotNil(t, manager.secretsClient)
		require.NotNil(t, manager.ssmClient)
	})

	t.Run("explicit endpoint and credentials", func(t *testing.T) {
		t.Setenv("AWS_ENDPOINT_URL", "http://localhost:5000")
		t.Setenv("AWS_REGION", "us-west-2")
		t.Setenv("AWS_ACCESS_KEY_ID", "key")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")

		manager := NewCachedSecretsManager()
		require.NotNil(t, manager)
		require.NotNil(t, manager.secretsClient)
		require.NotNil(t, manager.ssmClient)
	})

	t.Run("config load failure", func(t *testing.T) {
		originalLoadConfig := loadAWSConfig
		originalFatalf := fatalf
		defer func() {
			loadAWSConfig = originalLoadConfig
			fatalf = originalFatalf
		}()

		loadAWSConfig = func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
			return aws.Config{}, errors.New("load failed")
		}
		fatalf = func(format string, v ...interface{}) {
			panic("fatal called")
		}

		require.Panics(t, func() {
			_ = NewCachedSecretsManager()
		})
	})
}
