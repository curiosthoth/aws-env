package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/jmespath/go-jmespath"
	"log"
)

type CachedSecretsManager struct {
	rawCache      map[string]string
	jmesPathCache map[string]string

	client *secretsmanager.Client
}

func NewCachedSecretsManager() *CachedSecretsManager {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
	}
	return &CachedSecretsManager{
		client:        secretsmanager.NewFromConfig(cfg),
		rawCache:      make(map[string]string),
		jmesPathCache: make(map[string]string),
	}
}

// Get retrieves the secret value from AWS Secrets Manager
// The JMESPath can be nil, in which case the raw value is returned.
// Returns the value, a boolean indicating success, and an error message if applicable
func (v *CachedSecretsManager) Get(secretName string, jmesPath *string) (string, bool, string) {
	raw, found, errMsg := v.getRaw(secretName)
	if found {
		if jmesPath != nil {
			return v.getByJmesPath(secretName, raw, *jmesPath)
		} else {
			return raw, true, ""
		}
	} else {
		return "", false, errMsg
	}
}

// getRaw retrieves the secret value from AWS Secrets Manager and cache it locally as is
// Returns the value, a boolean indicating success, and an error message if applicable
func (v *CachedSecretsManager) getRaw(secretName string) (string, bool, string) {
	if r, found := v.rawCache[secretName]; found {
		return r, true, ""
	}
	input := &secretsmanager.GetSecretValueInput{
		SecretId: &secretName,
	}
	result, err := v.client.GetSecretValue(context.TODO(), input)
	if err != nil {
		return "", false, err.Error()
	}
	r := *result.SecretString
	v.rawCache[secretName] = r
	return r, true, ""
}

// getByJmesPath retrieves the secret value from AWS Secrets Manager and cache it locally
// Returns the value, a boolean indicating success, and an error message if applicable
func (v *CachedSecretsManager) getByJmesPath(secretName string, raw string, jmesPath string) (string, bool, string) {
	k := fmt.Sprintf("%s##%s", secretName, jmesPath)
	if r, found := v.jmesPathCache[k]; found {
		return r, true, ""
	} else {
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
		s := ""
		switch r.(type) {
		case string:
			// Do nothing
			s = r.(string)
		case float64:
			if r.(float64) == float64(int64(r.(float64))) {
				s = fmt.Sprintf("%d", int64(r.(float64)))
			} else {
				s = fmt.Sprintf("%f", r.(float64))
			}
		case bool:
			s = fmt.Sprintf("%t", r.(bool))
		default:
			marshal, err := json.Marshal(r)
			if err != nil {
				return "", false, fmt.Sprintf("failed to marshal result: %v", err)
			}
			s = string(marshal)
		}
		v.jmesPathCache[k] = s
		return s, true, ""
	}
}
