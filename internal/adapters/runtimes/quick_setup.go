package runtimes

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"picoclip/internal/core/domain"
)

const quickSetupProfileID = "openai-compatible"

func openAIQuickSetupSchema() domain.RuntimeQuickSetupSchema {
	return domain.RuntimeQuickSetupSchema{
		ProfileID:   quickSetupProfileID,
		Title:       "OpenAI-compatible provider",
		Description: "Configure one OpenAI-compatible endpoint and default model.",
		Fields: []domain.RuntimeQuickSetupField{
			{Name: "base_url", Label: "Base URL", Type: "url", Required: true, Placeholder: "https://provider.example/v1", Help: "Absolute HTTP(S) endpoint; local and private URLs are supported."},
			{Name: "api_key", Label: "API key", Type: "password", Help: "Leave blank to keep the current key."},
			{Name: "model", Label: "Model", Type: "text", Required: true, Placeholder: "model-id", Help: "Upstream model identifier."},
		},
	}
}

func validateOpenAICompatibleInput(baseURL, model string) error {
	if strings.TrimSpace(model) == "" {
		return fmt.Errorf("%w: model is required", domain.ErrInvalidInput)
	}
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("%w: base URL must be an absolute http or https URL", domain.ErrInvalidInput)
	}
	return nil
}

type namedBytes struct {
	name string
	data []byte
}

func configRevision(files ...namedBytes) string {
	h := sha256.New()
	for _, file := range files {
		h.Write([]byte(file.name))
		h.Write([]byte{0})
		h.Write(file.data)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func readConfigOrDefault(path string, fallback []byte) ([]byte, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return append([]byte(nil), fallback...), nil
	}
	return b, err
}

func decodeJSONObject(content []byte) (map[string]any, error) {
	var value map[string]any
	if err := json.Unmarshal(content, &value); err != nil {
		return nil, fmt.Errorf("invalid JSON configuration: %w", err)
	}
	if value == nil {
		return nil, errors.New("JSON configuration must be an object")
	}
	return value, nil
}

func marshalJSONObject(value map[string]any) ([]byte, error) {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func object(parent map[string]any, key string) (map[string]any, error) {
	if raw, ok := parent[key]; ok {
		value, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("configuration field %q must be an object", key)
		}
		return value, nil
	}
	value := map[string]any{}
	parent[key] = value
	return value, nil
}

func stringValue(parent map[string]any, key string) (string, error) {
	raw, ok := parent[key]
	if !ok {
		return "", nil
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("configuration field %q must be a string", key)
	}
	return value, nil
}

func atomicWriteFile(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".picoclip-config-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err = tmp.Chmod(mode); err == nil {
		_, err = tmp.Write(content)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func secureConfigMode(path string) os.FileMode {
	if info, err := os.Stat(path); err == nil && info.Mode().Perm()&0077 == 0 {
		return info.Mode().Perm()
	}
	return 0600
}

func atomicWritePairWithRollback(firstPath string, first []byte, firstMode os.FileMode, secondPath string, second []byte, secondMode os.FileMode, oldFirst []byte) error {
	if err := atomicWriteFile(firstPath, first, firstMode); err != nil {
		return err
	}
	if err := atomicWriteFile(secondPath, second, secondMode); err != nil {
		if rollbackErr := atomicWriteFile(firstPath, oldFirst, firstMode); rollbackErr != nil {
			return fmt.Errorf("second config write failed: %v; rollback failed: %w", err, rollbackErr)
		}
		return err
	}
	return nil
}

func decodeYAMLMap(content []byte) (map[string]any, error) {
	value := map[string]any{}
	if len(strings.TrimSpace(string(content))) == 0 {
		return value, nil
	}
	if err := yaml.Unmarshal(content, &value); err != nil {
		return nil, fmt.Errorf("invalid YAML configuration: %w", err)
	}
	return value, nil
}

func marshalYAMLMap(value map[string]any) ([]byte, error) { return yaml.Marshal(value) }

func requireRevision(input domain.RuntimeQuickSetupInput, current string) error {
	if input.Revision != current {
		return domain.ErrConfigurationChanged
	}
	if input.ProfileID != quickSetupProfileID {
		return fmt.Errorf("%w: unsupported profile", domain.ErrInvalidInput)
	}
	return nil
}

func secretUpdate(target map[string]any, key string, input domain.RuntimeQuickSetupInput) {
	if input.ClearAPIKey {
		delete(target, key)
	} else if input.APIKey != "" {
		target[key] = input.APIKey
	}
}

func existingHome() string { home, _ := os.UserHomeDir(); return home }
func xdgPath(env, fallback string) string {
	if value := os.Getenv(env); value != "" {
		return value
	}
	return filepath.Join(existingHome(), fallback)
}
