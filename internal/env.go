package internal

import (
	"fmt"
	"strings"
)

const (
	DefaultDelimiter     = "##"
	SecretsManagerScheme = "secretsmanager"
	SecretsManagerPrefix = SecretsManagerScheme + "://"
	SSMScheme            = "ssm"
	SSMPrefix            = SSMScheme + "://"
)

// EnvVar describes a single environment variable and its optional remote source.
type EnvVar struct {
	Name       string
	RawValue   string
	Scheme     *string
	SecretName *string
	JMESPath   *string
}

func (e EnvVar) String() string {
	schemeStr := "<nil>"
	if e.Scheme != nil {
		schemeStr = *e.Scheme
	}
	secretNameStr := "<nil>"
	if e.SecretName != nil {
		secretNameStr = *e.SecretName
	}
	jmesPathStr := "<nil>"
	if e.JMESPath != nil {
		jmesPathStr = *e.JMESPath
	}
	return fmt.Sprintf(
		"EnvVar{Name: %s, RawValue: %s, Scheme: %s, SecretName: %s, JMESPath: %s}",
		e.Name, e.RawValue, schemeStr, secretNameStr, jmesPathStr,
	)
}

// SplitEnvString parses an environment variable declaration and extracts any supported AWS-backed reference.
func SplitEnvString(env string, delimiter string) (envVar EnvVar, err error) {
	splitArr := strings.SplitN(env, "=", 2)
	if len(splitArr) != 2 {
		return envVar, fmt.Errorf("invalid env string. Need to be in the form of 'env=var'")
	}
	envVar.Name = splitArr[0]
	envVar.RawValue = strings.Trim(splitArr[1], " \t\n")
	if delimiter == "" {
		return envVar, fmt.Errorf("invalid delimiter. Need a non-empty delimiter")
	}

	switch {
	case strings.HasPrefix(envVar.RawValue, SecretsManagerPrefix):
		scheme := SecretsManagerScheme
		envVar.Scheme = &scheme
		return populateRemoteEnvVar(envVar, envVar.RawValue[len(SecretsManagerPrefix):], delimiter)
	case strings.HasPrefix(envVar.RawValue, SSMPrefix):
		scheme := SSMScheme
		envVar.Scheme = &scheme
		return populateRemoteEnvVar(envVar, envVar.RawValue[len(SSMPrefix):], delimiter)
	default:
		return envVar, nil
	}
}

// populateRemoteEnvVar parses the AWS resource name and optional nested JSON key path.
func populateRemoteEnvVar(envVar EnvVar, remoteValue string, delimiter string) (EnvVar, error) {
	keyArr := strings.Split(remoteValue, delimiter)
	if len(keyArr) == 0 || keyArr[0] == "" {
		return envVar, fmt.Errorf("invalid env string. Missing remote secret or parameter name")
	}
	envVar.SecretName = &keyArr[0]
	if len(keyArr) == 1 {
		return envVar, nil
	}

	pathSegments := keyArr[1:]
	for _, segment := range pathSegments {
		if segment == "" {
			return envVar, fmt.Errorf("invalid env string. JSON key path contains an empty segment")
		}
	}
	jmesPath := strings.Join(pathSegments, ".")
	envVar.JMESPath = &jmesPath
	return envVar, nil
}
