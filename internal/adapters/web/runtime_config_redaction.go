package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"gopkg.in/yaml.v3"
	"picoclip/internal/core/domain"
)

const redactedRuntimeSecret = "[REDACTED]"

func runtimeConfigRevision(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func redactRuntimeConfig(file domain.RuntimeConfigFile) []byte {
	var value any
	var err error
	switch strings.ToLower(file.Language) {
	case "json":
		err = json.Unmarshal(file.Content, &value)
	case "yaml", "yml":
		err = yaml.Unmarshal(file.Content, &value)
	default:
		return []byte(redactedRuntimeSecret)
	}
	if err != nil {
		return []byte(redactedRuntimeSecret)
	}
	redactRuntimeSecrets(value, false)
	if strings.EqualFold(file.Language, "json") {
		content, marshalErr := json.MarshalIndent(value, "", "  ")
		if marshalErr == nil {
			return append(content, '\n')
		}
	} else if content, marshalErr := yaml.Marshal(value); marshalErr == nil {
		return content
	}
	return []byte(redactedRuntimeSecret)
}

func redactRuntimeSecrets(value any, inherited bool) {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			secret := inherited || runtimeSecretKey(key)
			switch typed := child.(type) {
			case map[string]any, []any:
				redactRuntimeSecrets(typed, secret)
			default:
				if secret && typed != nil && typed != "" {
					current[key] = redactedRuntimeSecret
				}
			}
		}
	case []any:
		for i, child := range current {
			switch typed := child.(type) {
			case map[string]any, []any:
				redactRuntimeSecrets(typed, inherited)
			default:
				if inherited && typed != nil && typed != "" {
					current[i] = redactedRuntimeSecret
				}
			}
		}
	}
}

func runtimeSecretKey(key string) bool {
	compact := strings.ToLower(key)
	compact = strings.NewReplacer("-", "", "_", "", " ", "", ".", "").Replace(compact)
	return strings.Contains(compact, "apikey") || strings.HasSuffix(compact, "token") || strings.Contains(compact, "clientsecret") || strings.Contains(compact, "authorization") || strings.Contains(compact, "credential") || strings.Contains(compact, "password") || strings.Contains(compact, "passwd") || strings.Contains(compact, "cookie") || compact == "tokens" || compact == "secret" || compact == "secrets"
}

func restoreRedactedRuntimeConfig(original domain.RuntimeConfigFile, submitted []byte) ([]byte, error) {
	var before, after any
	if strings.EqualFold(original.Language, "json") {
		if err := json.Unmarshal(original.Content, &before); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(submitted, &after); err != nil {
			return nil, err
		}
		restoreRuntimeSecrets(after, before)
		content, err := json.MarshalIndent(after, "", "  ")
		return append(content, '\n'), err
	}
	if strings.EqualFold(original.Language, "yaml") || strings.EqualFold(original.Language, "yml") {
		if err := yaml.Unmarshal(original.Content, &before); err != nil {
			return nil, err
		}
		if err := yaml.Unmarshal(submitted, &after); err != nil {
			return nil, err
		}
		restoreRuntimeSecrets(after, before)
		return yaml.Marshal(after)
	}
	return submitted, nil
}

func restoreRuntimeSecrets(after, before any) {
	switch current := after.(type) {
	case map[string]any:
		previous, _ := before.(map[string]any)
		for key, child := range current {
			old := previous[key]
			if child == redactedRuntimeSecret {
				current[key] = old
				continue
			}
			restoreRuntimeSecrets(child, old)
		}
	case []any:
		previous, _ := before.([]any)
		for i, child := range current {
			var old any
			if i < len(previous) {
				old = previous[i]
			}
			if child == redactedRuntimeSecret {
				current[i] = old
				continue
			}
			restoreRuntimeSecrets(child, old)
		}
	}
}
