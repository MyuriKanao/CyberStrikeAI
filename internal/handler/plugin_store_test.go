package handler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cyberstrike-ai/internal/config"
)

func TestPluginStoreSettingsResponseRedactsGitHubToken(t *testing.T) {
	enabled := true
	resp := pluginStoreSettingsResponse(config.PluginStoreConfig{
		Enabled:     &enabled,
		RootDir:     "data/plugins",
		GitHubToken: "ghp_example",
	})

	if !resp.Enabled {
		t.Fatal("expected plugin store to be enabled")
	}
	if !resp.GitHubTokenConfigured {
		t.Fatal("expected github token to be reported as configured")
	}
	if resp.RootDir != "data/plugins" {
		t.Fatalf("root dir = %q", resp.RootDir)
	}
}

func TestUpdatePluginStoreConfigPersistsAndClearsGitHubToken(t *testing.T) {
	doc := newEmptyYAMLDocument()
	enabled := true
	updatePluginStoreConfig(doc, config.PluginStoreConfig{
		Enabled:     &enabled,
		RootDir:     "data/plugins",
		GitHubToken: "ghp_example",
	})

	storeNode := findMapValue(doc.Content[0], "plugin_store")
	if storeNode == nil {
		t.Fatal("plugin_store node not found")
	}
	tokenNode := findMapValue(storeNode, "github_token")
	if tokenNode == nil || tokenNode.Value != "ghp_example" {
		t.Fatalf("github_token node = %+v", tokenNode)
	}

	updatePluginStoreConfig(doc, config.PluginStoreConfig{
		Enabled:     &enabled,
		RootDir:     "data/plugins",
		GitHubToken: "",
	})
	tokenNode = findMapValue(storeNode, "github_token")
	if tokenNode == nil || tokenNode.Value != "" {
		t.Fatalf("github_token should be cleared, got %+v", tokenNode)
	}
}

func TestPluginStoreConfigBackupRedactsGitHubToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`plugin_store:
  enabled: true
  root_dir: data/plugins
  github_token: ghp_old_secret
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	enabled := true
	h := &PluginStoreHandler{
		configPath: path,
		config: &config.Config{PluginStore: config.PluginStoreConfig{
			Enabled:     &enabled,
			RootDir:     "data/plugins",
			GitHubToken: "",
		}},
	}

	if err := h.saveConfig(); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}
	backup, err := os.ReadFile(path + ".backup")
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if strings.Contains(string(backup), "ghp_old_secret") {
		t.Fatalf("backup should not contain old github token:\n%s", string(backup))
	}
	info, err := os.Stat(path + ".backup")
	if err != nil {
		t.Fatalf("stat backup: %v", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("backup should not be group/world readable, mode = %o", info.Mode().Perm())
	}
}
