package internal

import (
	"fmt"
	"strings"
)

const (
	SecretsManagerPrefix    = "secretsmanager://"
	SecretsManagerPrefixLen = len(SecretsManagerPrefix)
)

type EnvVar struct {
	Name       string
	RawValue   string
	SecretName *string // SecretName as stored in AWS SecretsManager
	JMESPath   *string
}

func (e EnvVar) String() string {
	secretNameStr := "<nil>"
	if e.SecretName != nil {
		secretNameStr = *e.SecretName
	}
	jmesPathStr := "<nil>"
	if e.JMESPath != nil {
		jmesPathStr = *e.JMESPath
	}
	return fmt.Sprintf(
		"EnvVar{Name: %s, RawValue: %s, SecretName: %s, JMESPath: %s}",
		e.Name, e.RawValue, secretNameStr, jmesPathStr,
	)
}

// SplitEnvString does things like the following:
// env1=val1  -> ["env1", "val1"], false
// env2=secretsmanager://val2  -> ["env2", "val2"], true
// env3=secretsmanager://val3##jmesPath -> ["env3", "val3", "jmesPath"], true
func SplitEnvString(env string) (envVar EnvVar, err error) {
	splitArr := strings.SplitN(env, "=", 2)
	if len(splitArr) != 2 {
		return envVar, fmt.Errorf("invalid env string. Need to be in the form of 'env=var'")
	}
	envVar.Name = splitArr[0]
	envVar.RawValue = strings.Trim(splitArr[1], " \t\n")

	// Check if it meets the format:
	// secretsmanager://<secret_name>...
	if strings.Index(envVar.RawValue, SecretsManagerPrefix) != 0 {
		return envVar, nil
	}

	keyArr := strings.SplitN(envVar.RawValue[SecretsManagerPrefixLen:], "##", 2)

	envVar.SecretName = &keyArr[0]
	if len(keyArr) == 1 {
		// Do nothing
	} else if len(keyArr) == 2 {
		// JSON
		envVar.JMESPath = &keyArr[1]
	} else {
		// We do not support deeper nested keys
		return envVar, fmt.Errorf("invalid env string. Only support one JMESPath seperated with '##'")
	}
	return envVar, nil
}
