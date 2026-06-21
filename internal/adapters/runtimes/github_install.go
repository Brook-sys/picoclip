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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int64  `json:"size"`
	} `json:"assets"`
}

func installFromGitHubRelease(ctx context.Context, owner string, repo string, assetPrefix string, binaryName string, dst string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+owner+"/"+repo+"/releases/latest", nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "picoclip-runtime-installer")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("github release request failed: %s", resp.Status)
	}
	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
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
	return []string{
		fmt.Sprintf("%s_%s_%s_%s%s", prefix, cleanVersion, titleOS(osName), arch, ext),
		fmt.Sprintf("%s_%s_%s%s", prefix, titleOS(osName), arch, ext),
	}
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
