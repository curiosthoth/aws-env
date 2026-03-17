package internal

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCachedSecretsManagerAgainstMoto(t *testing.T) {
	endpoint := defaultEndpoint()
	if endpoint == "" {
		t.Skip("AWS_ENDPOINT_URL is not set")
	}

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithRegion(defaultRegion()),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(defaultAccessKeyID(), defaultSecretAccessKey(), "")),
	)
	require.NoError(t, err)

	secretsClient := secretsmanager.NewFromConfig(cfg, func(o *secretsmanager.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
	ssmClient := ssm.NewFromConfig(cfg, func(o *ssm.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})

	secretName := "integration/secret"
	parameterName := "/integration/parameter"
	_, err = secretsClient.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name:         aws.String(secretName),
		SecretString: aws.String(`{"foo":{"bar":"secret-value"}}`),
	})
	require.NoError(t, err)

	_, err = ssmClient.PutParameter(ctx, &ssm.PutParameterInput{
		Name:      aws.String(parameterName),
		Type:      ssmtypes.ParameterTypeSecureString,
		Value:     aws.String(`{"foo":{"bar":"parameter-value"}}`),
		Overwrite: aws.Bool(true),
	})
	require.NoError(t, err)

	manager := NewCachedSecretsManager()

	value, found, errMsg := manager.Get(SecretsManagerScheme, secretName, ptr("foo.bar"))
	require.True(t, found)
	assert.Equal(t, "secret-value", value)
	assert.Empty(t, errMsg)

	value, found, errMsg = manager.Get(SSMScheme, parameterName, ptr("foo.bar"))
	require.True(t, found)
	assert.Equal(t, "parameter-value", value)
	assert.Empty(t, errMsg)

	value, found, errMsg = manager.Get(SecretsManagerScheme, secretName, nil)
	require.True(t, found)
	assert.Equal(t, `{"foo":{"bar":"secret-value"}}`, value)
	assert.Empty(t, errMsg)
}
