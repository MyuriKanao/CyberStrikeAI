package pluginstore

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func installRuntime(ctx context.Context, installedDir string, plugin PluginManifest, githubToken string) error {
	installType := strings.TrimSpace(plugin.Runtime.Install.Type)
	if installType == "" || installType == "none" {
		return nil
	}
	switch installType {
	case "github_release":
		return installGitHubRelease(ctx, installedDir, plugin, githubToken)
	case "python_venv":
		return installPythonVenv(ctx, installedDir, plugin)
	default:
		return fmt.Errorf("unsupported runtime install type %q", installType)
	}
}

func installGitHubRelease(ctx context.Context, installedDir string, plugin PluginManifest, githubToken string) error {
	repo := strings.TrimSpace(plugin.Runtime.Install.Repo)
	if repo == "" {
		return fmt.Errorf("github_release requires runtime.install.repo")
	}
	assetURL, assetName, err := resolveGitHubReleaseAsset(ctx, plugin.Runtime.Install, githubToken)
	if err != nil {
		return err
	}
	binDir, err := runtimeBinDir(installedDir, plugin)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	tmpFile := filepath.Join(os.TempDir(), safeName(plugin.ID)+"-"+safeName(assetName))
	if err := downloadFile(ctx, assetURL, tmpFile, githubToken); err != nil {
		return err
	}
	defer os.Remove(tmpFile)
	if err := verifyFileSHA256(tmpFile, plugin.Runtime.Install.SHA256); err != nil {
		return err
	}

	switch {
	case strings.HasSuffix(assetName, ".zip"):
		if err := extractZip(tmpFile, binDir); err != nil {
			return err
		}
	case strings.HasSuffix(assetName, ".tar.gz"), strings.HasSuffix(assetName, ".tgz"):
		if err := extractTarGz(tmpFile, binDir); err != nil {
			return err
		}
	default:
		name, err := githubReleaseBinaryName(plugin.Runtime.Install, assetName)
		if err != nil {
			return err
		}
		target := filepath.Join(binDir, name)
		if err := copyFile(tmpFile, target, 0o755); err != nil {
			return err
		}
	}
	return verifyRuntime(ctx, installedDir, plugin)
}

func githubReleaseBinaryName(install RuntimeInstall, assetName string) (string, error) {
	name := strings.TrimSpace(install.BinaryName)
	if name == "" {
		name = filepath.Base(assetName)
	}
	if name == "" || name == "." || filepath.IsAbs(name) || strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("unsafe github release binary_name %q", install.BinaryName)
	}
	return name, nil
}

type githubReleaseResponse struct {
	Assets []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func resolveGitHubReleaseAsset(ctx context.Context, install RuntimeInstall, githubToken string) (string, string, error) {
	version := strings.TrimSpace(install.Version)
	if version == "" || version == "latest" {
		version = "latest"
	}
	var apiURL string
	if version == "latest" {
		apiURL = "https://api.github.com/repos/" + strings.TrimSpace(install.Repo) + "/releases/latest"
	} else {
		apiURL = "https://api.github.com/repos/" + strings.TrimSpace(install.Repo) + "/releases/tags/" + version
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if auth := githubAuthorizationHeader(githubToken); auth != "" {
		req.Header.Set("Authorization", auth)
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", "", fmt.Errorf("github release API returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var release githubReleaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", "", err
	}
	asset := selectReleaseAsset(release.Assets, install.Asset)
	if asset.BrowserDownloadURL == "" {
		return "", "", fmt.Errorf("release asset %q not found", install.Asset)
	}
	return asset.BrowserDownloadURL, asset.Name, nil
}

func selectReleaseAsset(assets []githubReleaseAsset, wanted string) githubReleaseAsset {
	wanted = strings.TrimSpace(wanted)
	for _, asset := range assets {
		if asset.Name == wanted {
			return asset
		}
	}
	if wanted != "" {
		for _, asset := range assets {
			if strings.Contains(asset.Name, wanted) {
				return asset
			}
		}
		if i := strings.Index(wanted, "_"); i >= 0 {
			suffix := wanted[i:]
			for _, asset := range assets {
				if strings.HasSuffix(asset.Name, suffix) {
					return asset
				}
			}
		}
	}
	for _, asset := range assets {
		name := strings.ToLower(asset.Name)
		if strings.Contains(name, "linux_amd64") && (strings.HasSuffix(name, ".zip") || strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz")) {
			return asset
		}
	}
	return githubReleaseAsset{}
}

func downloadFile(ctx context.Context, url string, path string, githubToken string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if auth := githubDownloadAuthorization(url, githubToken); auth != "" {
		req.Header.Set("Authorization", auth)
	}
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download returned %s", resp.Status)
	}
	out, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

func githubDownloadAuthorization(rawURL string, githubToken string) string {
	if !isGitHubURL(rawURL) {
		return ""
	}
	return githubAuthorizationHeader(githubToken)
}

func verifyFileSHA256(path string, expected string) error {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return nil
	}
	expected = strings.TrimPrefix(expected, "sha256:")
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return err
	}
	actual := hex.EncodeToString(sum.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("sha256 mismatch for %s: expected %s, got %s", filepath.Base(path), expected, actual)
	}
	return nil
}

func installPythonVenv(ctx context.Context, installedDir string, plugin PluginManifest) error {
	specs, err := pythonInstallSpecs(plugin.Runtime.Install)
	if err != nil {
		return err
	}
	venvDir, err := safeJoin(installedDir, filepath.Join("runtime", "venv"))
	if err != nil {
		return err
	}
	binDir, err := runtimeBinDir(installedDir, plugin)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(venvDir); err != nil {
		return err
	}
	python, err := pythonExecutable()
	if err != nil {
		return err
	}
	if err := runCommand(ctx, "", python, "-m", "venv", venvDir); err != nil {
		return err
	}
	venvPython := filepath.Join(venvBinDir(venvDir), "python")
	pipArgs := append([]string{"-m", "pip", "install", "--disable-pip-version-check", "--no-cache-dir"}, specs...)
	if err := runCommand(ctx, "", venvPython, pipArgs...); err != nil {
		return err
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	for _, name := range pythonShimNames(plugin) {
		if err := writePythonVenvShim(binDir, name); err != nil {
			return err
		}
	}
	return verifyRuntime(ctx, installedDir, plugin)
}

func pythonInstallSpecs(install RuntimeInstall) ([]string, error) {
	var specs []string
	for _, pkg := range install.Packages {
		pkg = strings.TrimSpace(pkg)
		if pkg != "" {
			specs = append(specs, pkg)
		}
	}
	pkg := strings.TrimSpace(install.Package)
	if pkg == "" {
		return nil, fmt.Errorf("python_venv requires runtime.install.package")
	}
	version := strings.TrimSpace(install.Version)
	if version != "" && !pythonPackageHasVersion(pkg) {
		pkg += "==" + version
	}
	specs = append(specs, pkg)
	return specs, nil
}

func pythonPackageHasVersion(pkg string) bool {
	for _, marker := range []string{"==", ">=", "<=", "~=", "!=", ">", "<", "@", "git+"} {
		if strings.Contains(pkg, marker) {
			return true
		}
	}
	return false
}

func pythonExecutable() (string, error) {
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("python_venv requires python3 or python in PATH")
}

func venvBinDir(venvDir string) string {
	return filepath.Join(venvDir, "bin")
}

func pythonShimNames(plugin PluginManifest) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || filepath.IsAbs(name) || strings.ContainsAny(name, `/\`) {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	add(plugin.Runtime.Verify.Command)
	for _, name := range plugin.MCP.ExposeTools {
		add(name)
	}
	return out
}

func writePythonVenvShim(binDir string, name string) error {
	target, err := safeJoin(binDir, name)
	if err != nil {
		return err
	}
	script := `#!/bin/sh
DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
exec "$DIR/../venv/bin/` + name + `" "$@"
`
	return os.WriteFile(target, []byte(script), 0o755)
}

func runCommand(ctx context.Context, dir string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func extractZip(path string, dst string) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if f.FileInfo().Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("archive symlink %q is not allowed", f.Name)
		}
		target, err := safeJoin(dst, f.Name)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, err := f.Open()
		if err != nil {
			return err
		}
		mode := f.FileInfo().Mode().Perm()
		if mode == 0 {
			mode = 0o755
		}
		if err := writeReaderToFile(in, target, mode); err != nil {
			_ = in.Close()
			return err
		}
		_ = in.Close()
	}
	return nil
}

func extractTarGz(path string, dst string) error {
	file, err := os.Open(path)
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
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if hdr.FileInfo().IsDir() {
			continue
		}
		if hdr.Typeflag == tar.TypeSymlink || hdr.Typeflag == tar.TypeLink {
			return fmt.Errorf("archive link %q is not allowed", hdr.Name)
		}
		target, err := safeJoin(dst, hdr.Name)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		mode := hdr.FileInfo().Mode().Perm()
		if mode == 0 {
			mode = 0o755
		}
		if err := writeReaderToFile(tr, target, mode); err != nil {
			return err
		}
	}
}

func copyFile(src string, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	return writeReaderToFile(in, dst, mode)
}

func writeReaderToFile(in io.Reader, dst string, mode os.FileMode) error {
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func runtimeBinDir(installedDir string, plugin PluginManifest) (string, error) {
	rel := strings.TrimSpace(plugin.Runtime.Paths.Bin)
	if rel == "" {
		rel = filepath.Join("runtime", "bin")
	}
	return safeJoin(installedDir, rel)
}

func verifyRuntime(ctx context.Context, installedDir string, plugin PluginManifest) error {
	command := strings.TrimSpace(plugin.Runtime.Verify.Command)
	if command == "" {
		return nil
	}
	binDir, err := runtimeBinDir(installedDir, plugin)
	if err != nil {
		return err
	}
	exe := command
	if !filepath.IsAbs(exe) && !strings.ContainsAny(exe, `/\`) {
		candidate := filepath.Join(binDir, exe)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			exe = candidate
		}
	}
	cmd := exec.CommandContext(ctx, exe, plugin.Runtime.Verify.Args...)
	cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("runtime verify failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
