package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitEnvString(t *testing.T) {
	t.Run("plain env var", func(t *testing.T) {
		envVar, err := SplitEnvString("env1=val1", DefaultDelimiter)
		require.NoError(t, err)
		assert.Equal(t, "env1", envVar.Name)
		assert.Equal(t, "val1", envVar.RawValue)
		assert.Nil(t, envVar.Scheme)
		assert.Nil(t, envVar.SecretName)
		assert.Nil(t, envVar.JMESPath)
	})

	t.Run("secrets manager raw lookup", func(t *testing.T) {
		envVar, err := SplitEnvString("env2=secretsmanager://app/secret", DefaultDelimiter)
		require.NoError(t, err)
		require.NotNil(t, envVar.Scheme)
		require.NotNil(t, envVar.SecretName)
		assert.Equal(t, SecretsManagerScheme, *envVar.Scheme)
		assert.Equal(t, "app/secret", *envVar.SecretName)
		assert.Nil(t, envVar.JMESPath)
	})

	t.Run("ssm nested lookup with custom delimiter", func(t *testing.T) {
		envVar, err := SplitEnvString("env3=ssm://app/config::foo::bar", "::")
		require.NoError(t, err)
		require.NotNil(t, envVar.Scheme)
		require.NotNil(t, envVar.SecretName)
		require.NotNil(t, envVar.JMESPath)
		assert.Equal(t, SSMScheme, *envVar.Scheme)
		assert.Equal(t, "app/config", *envVar.SecretName)
		assert.Equal(t, "foo.bar", *envVar.JMESPath)
	})

	t.Run("invalid env string", func(t *testing.T) {
		_, err := SplitEnvString("missing-delimiter", DefaultDelimiter)
		require.Error(t, err)
	})

	t.Run("empty remote name", func(t *testing.T) {
		_, err := SplitEnvString("env4=secretsmanager://##foo", DefaultDelimiter)
		require.Error(t, err)
	})

	t.Run("empty nested segment", func(t *testing.T) {
		_, err := SplitEnvString("env5=ssm://param####foo", DefaultDelimiter)
		require.Error(t, err)
	})

	t.Run("empty delimiter rejected", func(t *testing.T) {
		_, err := SplitEnvString("env6=secretsmanager://app/secret", "")
		require.Error(t, err)
	})
}

func TestEnvVarString(t *testing.T) {
	scheme := SecretsManagerScheme
	name := "secret/name"
	path := "foo.bar"

	assert.Contains(t, (EnvVar{
		Name:       "TEST_VAR",
		RawValue:   "secretsmanager://secret/name##foo##bar",
		Scheme:     &scheme,
		SecretName: &name,
		JMESPath:   &path,
	}).String(), "Scheme: secretsmanager")
}
