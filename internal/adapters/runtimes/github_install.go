package runtimes

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"picoclip/internal/core/domain"
)

type githubRelease struct {
	TagName    string    `json:"tag_name"`
	Prerelease bool      `json:"prerelease"`
	CreatedAt  time.Time `json:"created_at"`
	Assets     []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int64  `json:"size"`
	} `json:"assets"`
}

var githubHTTPClient = &http.Client{Timeout: 10 * time.Second}
var githubRedirectHTTPClient = &http.Client{
	Timeout: 10 * time.Second,
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

type githubAPIError struct {
	Status  string
	Message string
	Reset   string
}

func (e *githubAPIError) Error() string {
	message := strings.TrimSpace(e.Message)
	if message == "" {
		message = "GitHub API request failed"
	}
	if e.Reset != "" {
		return fmt.Sprintf("%s: %s (rate limit resets at %s)", e.Status, message, e.Reset)
	}
	return fmt.Sprintf("%s: %s", e.Status, message)
}

func prepareGitHubRequest(req *http.Request) {
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "picoclip-runtime-installer")
	token := strings.TrimSpace(os.Getenv("GH_TOKEN"))
	if token == "" {
		token = strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func githubResponseError(resp *http.Response) error {
	var payload struct {
		Message string `json:"message"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&payload)
	return &githubAPIError{Status: resp.Status, Message: payload.Message, Reset: resp.Header.Get("X-RateLimit-Reset")}
}

func resolveGitHubReleaseTag(ctx context.Context, owner, repo, versionAlias string) (string, error) {
	versionAlias = strings.TrimSpace(versionAlias)
	if versionAlias != "" && versionAlias != "latest" {
		return versionAlias, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://github.com/%s/%s/releases/latest", url.PathEscape(owner), url.PathEscape(repo)), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "picoclip-runtime-installer")
	resp, err := githubRedirectHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		return "", fmt.Errorf("github latest release redirect failed: %s", resp.Status)
	}
	location := resp.Header.Get("Location")
	parsed, err := url.Parse(location)
	if err != nil {
		return "", fmt.Errorf("invalid github latest release redirect: %w", err)
	}
	const marker = "/releases/tag/"
	index := strings.Index(parsed.Path, marker)
	if index < 0 {
		return "", fmt.Errorf("github latest release redirect did not contain a tag")
	}
	tag, err := url.PathUnescape(strings.TrimPrefix(parsed.Path[index:], marker))
	if err != nil || strings.TrimSpace(tag) == "" {
		return "", fmt.Errorf("github latest release redirect contained an invalid tag")
	}
	return tag, nil
}

func listGitHubVersions(ctx context.Context, owner, repo string, limit int) ([]domain.RuntimeVersion, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?per_page=%d", owner, repo, limit), nil)
	if err != nil {
		return nil, err
	}
	prepareGitHubRequest(req)
	resp, err := githubHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, githubResponseError(resp)
	}

	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}

	var versions []domain.RuntimeVersion
	var foundLatest bool
	for _, rel := range releases {
		version := domain.RuntimeVersion{
			Tag:        rel.TagName,
			Label:      rel.TagName,
			Prerelease: rel.Prerelease,
			CreatedAt:  rel.CreatedAt,
		}
		if !rel.Prerelease && !foundLatest {
			version.Latest = true
			version.Label = "latest (" + rel.TagName + ")"
			foundLatest = true
		}
		if rel.Prerelease {
			version.Nightly = true
		}
		versions = append(versions, version)
	}
	return versions, nil
}

func fetchGitHubRelease(ctx context.Context, owner, repo, versionAlias string) (githubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)

	if versionAlias != "" && versionAlias != "latest" {
		if versionAlias == "nightly" {
			url = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?per_page=1", owner, repo)
		} else {
			url = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", owner, repo, versionAlias)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return githubRelease{}, err
	}
	prepareGitHubRequest(req)
	resp, err := githubHTTPClient.Do(req)
	if err != nil {
		return githubRelease{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound && versionAlias != "" && versionAlias != "latest" && versionAlias != "nightly" {
		return githubRelease{}, fmt.Errorf("Version tag %q was not found for %s. Check the tag name or choose one from the suggestions.", versionAlias, repo)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return githubRelease{}, githubResponseError(resp)
	}

	if versionAlias == "nightly" {
		var releases []githubRelease
		if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
			return githubRelease{}, err
		}
		for _, rel := range releases {
			if rel.Prerelease {
				return rel, nil
			}
		}
		return githubRelease{}, fmt.Errorf("no nightly/prerelease found for %s", repo)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return githubRelease{}, err
	}
	return release, nil
}

func installFromGitHubRelease(ctx context.Context, owner string, repo string, assetPrefix string, binaryName string, versionAlias string, dst string) (string, string, error) {
	if versionAlias != "nightly" {
		tag, err := resolveGitHubReleaseTag(ctx, owner, repo, versionAlias)
		if err != nil {
			return "", "", err
		}
		assetNames := runtimeAssetNames(assetPrefix, tag)
		var failures []string
		for _, assetName := range assetNames {
			downloadURL := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(tag), url.PathEscape(assetName))
			if err := downloadAndExtract(ctx, downloadURL, assetName, binaryName, dst); err == nil {
				return tag, downloadURL, nil
			} else {
				failures = append(failures, assetName+": "+err.Error())
			}
		}
		return "", "", fmt.Errorf("release asset download failed; tried %s", strings.Join(failures, "; "))
	}

	release, err := fetchGitHubRelease(ctx, owner, repo, versionAlias)
	if err != nil {
		return "", "", err
	}

	assetNames := runtimeAssetNames(assetPrefix, release.TagName)
	for _, expected := range assetNames {
		for _, asset := range release.Assets {
			if asset.Name == expected {
				if asset.Size > 250*1024*1024 {
					return "", "", errors.New("runtime asset is too large")
				}
				if err := downloadAndExtract(ctx, asset.BrowserDownloadURL, asset.Name, binaryName, dst); err != nil {
					return "", "", err
				}
				return release.TagName, asset.BrowserDownloadURL, nil
			}
		}
	}
	return "", "", fmt.Errorf("release asset not found, tried %s", strings.Join(assetNames, ", "))
}

func runtimeAssetNames(prefix string, version string) []string {
	osName := runtime.GOOS
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x86_64"
	}
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	cleanVersion := strings.TrimPrefix(version, "v")
	names := []string{
		fmt.Sprintf("%s_%s_%s_%s%s", prefix, cleanVersion, titleOS(osName), arch, ext),
		fmt.Sprintf("%s_%s_%s%s", prefix, titleOS(osName), arch, ext),
	}
	for _, lowerOS := range lowerOSNames(osName) {
		for _, candidateArch := range archNames(arch) {
			names = append(names,
				fmt.Sprintf("%s-%s-%s%s", prefix, lowerOS, candidateArch, ext),
				fmt.Sprintf("%s-%s%s", prefix, candidateArch, ext),
			)
		}
	}
	return names
}

func titleOS(osName string) string {
	switch osName {
	case "darwin":
		return "Darwin"
	case "linux":
		return "Linux"
	case "windows":
		return "Windows"
	default:
		return osName
	}
}

func lowerOSNames(osName string) []string {
	if osName == "darwin" {
		return []string{"darwin", "macos"}
	}
	return []string{osName}
}

func archNames(arch string) []string {
	if arch == "arm64" {
		return []string{"arm64", "aarch64"}
	}
	return []string{arch}
}

func downloadAndExtract(ctx context.Context, url string, assetName string, binaryName string, dst string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "picoclip-runtime-installer")
	client := http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("runtime download failed: %s", resp.Status)
	}
	tmpDir, err := os.MkdirTemp("", "picoclip-runtime-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	archivePath := filepath.Join(tmpDir, assetName)
	file, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, io.LimitReader(resp.Body, 250*1024*1024)); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if strings.HasSuffix(assetName, ".zip") {
		return extractZipBinary(archivePath, binaryName, dst)
	}
	return extractTarGzBinary(archivePath, binaryName, dst)
}

func extractZipBinary(archivePath string, binaryName string, dst string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if isRuntimeBinary(f.Name, binaryName) {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()
			return writeExecutable(dst, rc)
		}
	}
	return fmt.Errorf("binary %s not found in archive", binaryName)
}

func extractTarGzBinary(archivePath string, binaryName string, dst string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if header.Typeflag == tar.TypeReg && isRuntimeBinary(header.Name, binaryName) {
			return writeExecutable(dst, tr)
		}
	}
	return fmt.Errorf("binary %s not found in archive", binaryName)
}

func isRuntimeBinary(name string, binaryName string) bool {
	base := filepath.Base(name)
	return base == binaryName || base == binaryName+".exe"
}

func writeExecutable(dst string, r io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, r); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
