package pluginstore

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (m *Manager) AddOrSyncSource(ctx context.Context, name string, sourceURL string) (Source, error) {
	if err := m.Ensure(); err != nil {
		return Source{}, err
	}
	sourceURL = strings.TrimSpace(sourceURL)
	if sourceURL == "" {
		return Source{}, fmt.Errorf("source url is required")
	}
	cleanURL, err := cleanSourceURL(sourceURL)
	if err != nil {
		return Source{}, err
	}
	sourceURL = cleanURL
	if strings.TrimSpace(name) == "" {
		name = deriveSourceName(sourceURL)
	}
	name = safeName(name)
	target := m.sourceDir(name)
	if err := syncSource(ctx, sourceURL, target, m.githubToken); err != nil {
		return Source{}, err
	}
	src := Source{Name: name, URL: sourceURL, Path: m.registrySourcePath(name), UpdatedAt: time.Now()}
	reg, err := m.LoadRegistry()
	if err != nil {
		return Source{}, err
	}
	upsertSource(&reg, src)
	if err := m.SaveRegistry(reg); err != nil {
		return Source{}, err
	}
	return src, nil
}

func (m *Manager) CatalogForSource(name string) (*Catalog, error) {
	reg, err := m.LoadRegistry()
	if err != nil {
		return nil, err
	}
	for _, src := range reg.Sources {
		if src.Name == name {
			cat, err := LoadCatalog(m.resolveRegistryPath(src.Path))
			if err != nil {
				return nil, err
			}
			for i := range cat.Plugins {
				cat.Plugins[i].SourceName = src.Name
				cat.Plugins[i].SourceURL = src.URL
			}
			m.annotateCatalogAvailability(cat)
			return cat, nil
		}
	}
	return nil, fmt.Errorf("plugin source %q not found", name)
}

func (m *Manager) Catalogs() ([]Catalog, error) {
	reg, err := m.LoadRegistry()
	if err != nil {
		return nil, err
	}
	out := make([]Catalog, 0, len(reg.Sources))
	for _, src := range reg.Sources {
		cat, err := LoadCatalog(m.resolveRegistryPath(src.Path))
		if err != nil {
			return nil, fmt.Errorf("source %q: %w", src.Name, err)
		}
		for i := range cat.Plugins {
			cat.Plugins[i].SourceName = src.Name
			cat.Plugins[i].SourceURL = src.URL
		}
		m.annotateCatalogAvailability(cat)
		out = append(out, *cat)
	}
	return out, nil
}

func (m *Manager) annotateCatalogAvailability(cat *Catalog) {
	if m == nil || cat == nil {
		return
	}
	for i := range cat.Plugins {
		toolNames, err := loadToolNames(cat.Plugins[i].Directory, cat.Plugins[i].Tools)
		if err != nil {
			continue
		}
		conflicts := m.ToolNameConflicts(cat.Plugins[i].ID, toolNames)
		if len(conflicts) == 0 {
			cat.Plugins[i].InstallState = "available"
			continue
		}
		cat.Plugins[i].InstallState = "conflict"
		cat.Plugins[i].ConflictTools = conflicts
	}
}

func syncSource(ctx context.Context, sourceURL string, target string, githubToken string) error {
	if localPath, ok := localSourcePath(sourceURL); ok {
		return copyDir(localPath, target)
	}
	if _, err := os.Stat(filepath.Join(target, ".git")); err == nil {
		if current, err := gitRemoteURL(ctx, target); err == nil && gitRemoteNeedsReclone(current, sourceURL) {
			if err := os.RemoveAll(target); err != nil {
				return err
			}
		} else {
			cmd := gitCommand(ctx, githubToken, sourceURL, "-C", target, "pull", "--ff-only")
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("git pull: %w: %s", err, strings.TrimSpace(string(out)))
			}
			return nil
		}
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	cmd := gitCommand(ctx, githubToken, sourceURL, "clone", "--depth=1", sourceURL, target)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func gitCommand(ctx context.Context, githubToken string, sourceURL string, args ...string) *exec.Cmd {
	gitArgs := append(gitAuthArgs(githubToken, sourceURL), args...)
	return exec.CommandContext(ctx, gitExecutable(), gitArgs...)
}

func gitRemoteURL(ctx context.Context, target string) (string, error) {
	cmd := exec.CommandContext(ctx, gitExecutable(), "-C", target, "remote", "get-url", "origin")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git remote get-url origin: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func gitRemoteNeedsReclone(current string, requested string) bool {
	return strings.TrimSpace(current) != strings.TrimSpace(requested)
}

func gitAuthArgs(githubToken string, sourceURL string) []string {
	token := strings.TrimSpace(githubToken)
	if token == "" || !isGitHubURL(sourceURL) {
		return nil
	}
	return []string{"-c", "http.https://github.com/.extraHeader=Authorization: " + githubAuthorizationHeader(token)}
}

func githubAuthorizationHeader(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	encoded := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	return "Basic " + encoded
}

func isGitHubURL(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Hostname(), "github.com")
}

func cleanSourceURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", fmt.Errorf("source url is required")
	}
	u, err := url.Parse(rawURL)
	if err == nil && u.Host != "" {
		if u.User != nil {
			return "", fmt.Errorf("source url must not contain embedded credentials; use the GitHub Token field instead")
		}
		return u.String(), nil
	}
	return rawURL, nil
}

func sanitizeRegistrySourceURLs(reg *Registry) {
	if reg == nil {
		return
	}
	for i := range reg.Sources {
		reg.Sources[i].URL = stripSourceURLUserinfo(reg.Sources[i].URL)
	}
}

func stripSourceURLUserinfo(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" || u.User == nil {
		return rawURL
	}
	u.User = nil
	return u.String()
}

func gitExecutable() string {
	if path, err := exec.LookPath("git"); err == nil {
		return path
	}
	for _, path := range []string{"/usr/bin/git", "/usr/local/bin/git", "/bin/git"} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0 {
			return path
		}
	}
	return "git"
}

func localSourcePath(sourceURL string) (string, bool) {
	if strings.HasPrefix(sourceURL, "file://") {
		u, err := url.Parse(sourceURL)
		if err != nil {
			return "", false
		}
		if _, err := os.Stat(u.Path); err == nil {
			return u.Path, true
		}
	}
	if filepath.IsAbs(sourceURL) || strings.HasPrefix(sourceURL, ".") {
		if info, err := os.Stat(sourceURL); err == nil && info.IsDir() {
			return sourceURL, true
		}
	}
	return "", false
}

func deriveSourceName(sourceURL string) string {
	sourceURL = strings.TrimSuffix(strings.TrimSpace(sourceURL), "/")
	if u, err := url.Parse(sourceURL); err == nil && u.Host != "" {
		base := strings.TrimSuffix(filepath.Base(u.Path), ".git")
		if base != "" && base != "." {
			return base
		}
		return u.Host
	}
	base := strings.TrimSuffix(filepath.Base(sourceURL), ".git")
	if base == "" || base == "." {
		return "source"
	}
	return base
}
