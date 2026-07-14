package runtimes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"picoclip/internal/core/domain"
)

const quickSetupProfileID = "openai-compatible"

var modelTestHTTPClient = newModelTestHTTPClient(
	net.DefaultResolver.LookupNetIP,
	(&net.Dialer{}).DialContext,
)
var modelTestURLValidator = validatePublicModelTestURL

type modelTestLookupFunc func(context.Context, string, string) ([]netip.Addr, error)
type modelTestDialFunc func(context.Context, string, string) (net.Conn, error)

func newModelTestHTTPClient(lookup modelTestLookupFunc, dial modelTestDialFunc) *http.Client {
	return &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				return dialPublicModelTestHost(ctx, network, address, lookup, dial)
			},
		},
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("model test exceeded redirect limit")
			}
			return validatePublicModelTestURL(request.URL)
		},
	}
}

func validatePublicModelTestURL(endpoint *url.URL) error {
	if endpoint == nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" || endpoint.User != nil {
		return fmt.Errorf("model test endpoint must be an absolute public HTTP(S) URL without credentials")
	}
	if port := endpoint.Port(); port != "" && !((endpoint.Scheme == "http" && port == "80") || (endpoint.Scheme == "https" && port == "443")) {
		return fmt.Errorf("model test endpoint must use port 80 or 443")
	}
	return nil
}

func dialPublicModelTestHost(ctx context.Context, network, address string, lookup modelTestLookupFunc, dial modelTestDialFunc) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addresses, err := lookup(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	for _, resolved := range addresses {
		if !isPublicModelTestIP(resolved) {
			return nil, fmt.Errorf("model test endpoint must resolve to a public address")
		}
		connection, err := dial(ctx, network, net.JoinHostPort(resolved.String(), port))
		if err == nil {
			return connection, nil
		}
	}
	return nil, fmt.Errorf("connect to model test endpoint: no public address accepted the connection")
}

var blockedModelTestPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001:db8::/32"),
}

func isPublicModelTestIP(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsMulticast() || address.IsUnspecified() {
		return false
	}
	for _, blocked := range blockedModelTestPrefixes {
		if blocked.Contains(address) {
			return false
		}
	}
	return true
}

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

type fileSnapshot struct {
	content []byte
	mode    os.FileMode
	exists  bool
}

func snapshotFile(path string) (fileSnapshot, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return fileSnapshot{}, nil
	}
	if err != nil {
		return fileSnapshot{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return fileSnapshot{}, err
	}
	return fileSnapshot{content: content, mode: info.Mode().Perm(), exists: true}, nil
}

func restoreFile(path string, snapshot fileSnapshot) error {
	if !snapshot.exists {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	return atomicWriteFile(path, snapshot.content, snapshot.mode)
}

func atomicWritePairWithRollback(firstPath string, first []byte, firstMode os.FileMode, secondPath string, second []byte, secondMode os.FileMode) error {
	firstBefore, err := snapshotFile(firstPath)
	if err != nil {
		return err
	}
	secondBefore, err := snapshotFile(secondPath)
	if err != nil {
		return err
	}
	if err := atomicWriteFile(firstPath, first, firstMode); err != nil {
		return err
	}
	if err := atomicWriteFile(secondPath, second, secondMode); err != nil {
		firstRollback := restoreFile(firstPath, firstBefore)
		secondRollback := restoreFile(secondPath, secondBefore)
		if firstRollback != nil || secondRollback != nil {
			return fmt.Errorf("second config write failed: %v; rollback failed: first=%v second=%v", err, firstRollback, secondRollback)
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

func testOpenAICompatibleModel(ctx context.Context, baseURL, apiKey, model string) (domain.RuntimeModelTestResult, error) {
	if err := validateOpenAICompatibleInput(baseURL, model); err != nil {
		return domain.RuntimeModelTestResult{}, err
	}
	parsedBaseURL, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || modelTestURLValidator(parsedBaseURL) != nil {
		return domain.RuntimeModelTestResult{}, fmt.Errorf("%w: Test Model requires a public HTTP(S) endpoint on port 80 or 443", domain.ErrInvalidInput)
	}
	payload, err := json.Marshal(map[string]any{
		"model":      strings.TrimSpace(model),
		"messages":   []map[string]string{{"role": "user", "content": "Say exactly and only the word PONG"}},
		"max_tokens": 8,
	})
	if err != nil {
		return domain.RuntimeModelTestResult{}, err
	}
	endpoint := strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/chat/completions"
	testCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(testCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return domain.RuntimeModelTestResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	started := time.Now()
	// modelTestHTTPClient disables proxies, validates every redirect, resolves DNS in
	// DialContext, rejects special/private addresses, and dials only the validated IP.
	response, err := modelTestHTTPClient.Do(req)
	latency := time.Since(started)
	checkedAt := time.Now().UTC()
	if err != nil {
		return domain.RuntimeModelTestResult{Status: "error", Message: "Model request failed", Latency: latency, CheckedAt: checkedAt}, nil
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return domain.RuntimeModelTestResult{Status: "error", Message: "Could not read model response", Latency: latency, CheckedAt: checkedAt}, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return domain.RuntimeModelTestResult{Status: "error", Message: fmt.Sprintf("Provider returned HTTP %d", response.StatusCode), Latency: latency, CheckedAt: checkedAt}, nil
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Text string `json:"text"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil || len(decoded.Choices) == 0 {
		return domain.RuntimeModelTestResult{Status: "error", Message: "Provider returned an invalid OpenAI-compatible response", Latency: latency, CheckedAt: checkedAt}, nil
	}
	output := strings.TrimSpace(decoded.Choices[0].Message.Content)
	if output == "" {
		output = strings.TrimSpace(decoded.Choices[0].Text)
	}
	if len(output) > 500 {
		output = output[:500]
	}
	return domain.RuntimeModelTestResult{Status: "ok", Message: "Model responded successfully", Output: output, Latency: latency, CheckedAt: checkedAt}, nil
}

func existingHome() string { home, _ := os.UserHomeDir(); return home }
func xdgPath(env, fallback string) string {
	if value := os.Getenv(env); value != "" {
		return value
	}
	return filepath.Join(existingHome(), fallback)
}
