package pluginstore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallPluginPersistsRegistry(t *testing.T) {
	repo := writeFixtureRepository(t)
	manager := New(filepath.Join(t.TempDir(), "plugins"))

	source, err := manager.AddOrSyncSource(context.Background(), "fixture", repo)
	if err != nil {
		t.Fatalf("AddOrSyncSource: %v", err)
	}
	if filepath.IsAbs(source.Path) {
		t.Fatalf("source path should be stored relative to the plugin root, got %q", source.Path)
	}
	installed, err := manager.InstallPlugin(context.Background(), source.Name, "nuclei")
	if err != nil {
		t.Fatalf("InstallPlugin: %v", err)
	}
	if filepath.IsAbs(installed.InstalledDir) {
		t.Fatalf("installed dir should be stored relative to the plugin root, got %q", installed.InstalledDir)
	}
	if !installed.Enabled {
		t.Fatal("installed plugin should be enabled")
	}
	if len(installed.ToolNames) != 1 || installed.ToolNames[0] != "nuclei" {
		t.Fatalf("tool names = %+v", installed.ToolNames)
	}

	reloaded := New(manager.RootDir())
	items, err := reloaded.ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled: %v", err)
	}
	if len(items) != 1 || items[0].ID != "nuclei" || !items[0].Enabled {
		t.Fatalf("unexpected installed registry: %+v", items)
	}
	if filepath.IsAbs(items[0].InstalledDir) {
		t.Fatalf("reloaded installed dir should be relative, got %q", items[0].InstalledDir)
	}
}

func TestLoadInstalledTools(t *testing.T) {
	repo := writeFixtureRepository(t)
	manager := New(filepath.Join(t.TempDir(), "plugins"))
	source, err := manager.AddOrSyncSource(context.Background(), "fixture", repo)
	if err != nil {
		t.Fatalf("AddOrSyncSource: %v", err)
	}
	if _, err := manager.InstallPlugin(context.Background(), source.Name, "nuclei"); err != nil {
		t.Fatalf("InstallPlugin: %v", err)
	}

	tools, err := manager.LoadInstalledTools()
	if err != nil {
		t.Fatalf("LoadInstalledTools: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("tool count = %d", len(tools))
	}
	if tools[0].Name != "nuclei" || tools[0].Command != "nuclei" || !tools[0].Enabled {
		t.Fatalf("unexpected tool: %+v", tools[0])
	}
}

func TestRuntimeBinDirsIncludesCustomPluginBinPath(t *testing.T) {
	repo := writeFixtureRepository(t)
	manifestPath := filepath.Join(repo, "plugins", "nuclei", "plugin.yaml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	data = []byte(strings.Replace(string(data), "runtime:\n  install:", "runtime:\n  paths:\n    bin: runtime/custom-bin\n  install:", 1))
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	manager := New(filepath.Join(t.TempDir(), "plugins"))
	source, err := manager.AddOrSyncSource(context.Background(), "fixture", repo)
	if err != nil {
		t.Fatalf("AddOrSyncSource: %v", err)
	}
	if _, err := manager.InstallPlugin(context.Background(), source.Name, "nuclei"); err != nil {
		t.Fatalf("InstallPlugin: %v", err)
	}

	want := filepath.Join(manager.installedPluginDir("nuclei"), "runtime", "custom-bin")
	for _, dir := range manager.RuntimeBinDirs() {
		if dir == want {
			return
		}
	}
	t.Fatalf("RuntimeBinDirs did not include custom bin %q: %+v", want, manager.RuntimeBinDirs())
}

func TestInstallPluginRejectsExposeToolMismatch(t *testing.T) {
	repo := writeFixtureRepository(t)
	manifestPath := filepath.Join(repo, "plugins", "nuclei", "plugin.yaml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	data = []byte(strings.Replace(string(data), "expose_tools: [nuclei]", "expose_tools: [nuclei_missing]", 1))
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	manager := New(filepath.Join(t.TempDir(), "plugins"))
	source, err := manager.AddOrSyncSource(context.Background(), "fixture", repo)
	if err != nil {
		t.Fatalf("AddOrSyncSource: %v", err)
	}
	if _, err := manager.InstallPlugin(context.Background(), source.Name, "nuclei"); err == nil {
		t.Fatal("expected expose_tools mismatch to fail installation")
	} else if !strings.Contains(err.Error(), "mcp.expose_tools") {
		t.Fatalf("expected mcp.expose_tools error, got %v", err)
	}
	if _, err := os.Stat(manager.installedPluginDir("nuclei")); !os.IsNotExist(err) {
		t.Fatalf("mismatched plugin should not leave an installed directory, stat err = %v", err)
	}
}

func TestAddOrSyncSourceRejectsURLUserinfo(t *testing.T) {
	manager := New(filepath.Join(t.TempDir(), "plugins"))

	_, err := manager.AddOrSyncSource(context.Background(), "private", "https://ghp_secret@github.com/example/CyberStrikeAI-Plugins.git")
	if err == nil {
		t.Fatal("expected embedded source credentials to be rejected")
	}
	if !strings.Contains(err.Error(), "GitHub Token") {
		t.Fatalf("expected GitHub Token guidance, got %v", err)
	}
}

func TestLoadRegistryStripsSourceURLUserinfo(t *testing.T) {
	manager := New(filepath.Join(t.TempDir(), "plugins"))
	if err := manager.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	raw := Registry{
		Sources: []Source{{
			Name: "old-private",
			URL:  "https://ghp_secret@github.com/example/CyberStrikeAI-Plugins.git",
			Path: "sources/old-private",
		}},
	}
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal registry: %v", err)
	}
	if err := os.WriteFile(manager.registryPath, data, 0o644); err != nil {
		t.Fatalf("write registry: %v", err)
	}

	reg, err := manager.LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if len(reg.Sources) != 1 {
		t.Fatalf("source count = %d", len(reg.Sources))
	}
	if strings.Contains(reg.Sources[0].URL, "ghp_secret") || strings.Contains(reg.Sources[0].URL, "@github.com") {
		t.Fatalf("source url was not sanitized: %q", reg.Sources[0].URL)
	}
}

func TestGitRemoteNeedsRecloneWhenSourceURLChanges(t *testing.T) {
	if !gitRemoteNeedsReclone("https://github.com/example/old.git", "https://github.com/example/new.git") {
		t.Fatal("changed source URL should require a fresh clone")
	}
	if gitRemoteNeedsReclone(" https://github.com/example/repo.git ", "https://github.com/example/repo.git") {
		t.Fatal("same source URL with surrounding whitespace should not require a fresh clone")
	}
}

func TestInstallPluginRejectsReservedToolNameConflict(t *testing.T) {
	repo := writeFixtureRepository(t)
	manager := New(filepath.Join(t.TempDir(), "plugins"))
	manager.SetReservedToolNames([]string{"nuclei"})
	source, err := manager.AddOrSyncSource(context.Background(), "fixture", repo)
	if err != nil {
		t.Fatalf("AddOrSyncSource: %v", err)
	}

	if _, err := manager.InstallPlugin(context.Background(), source.Name, "nuclei"); err == nil {
		t.Fatal("expected reserved tool name conflict to fail installation")
	} else if !strings.Contains(err.Error(), "tool name conflicts") {
		t.Fatalf("expected tool name conflict error, got %v", err)
	}
	if _, err := os.Stat(manager.installedPluginDir("nuclei")); !os.IsNotExist(err) {
		t.Fatalf("conflicting plugin should not leave an installed directory, stat err = %v", err)
	}
}

func TestInstallPluginRejectsInstalledPluginToolNameConflict(t *testing.T) {
	repo := writeFixtureRepository(t)
	manager := New(filepath.Join(t.TempDir(), "plugins"))
	source, err := manager.AddOrSyncSource(context.Background(), "fixture", repo)
	if err != nil {
		t.Fatalf("AddOrSyncSource: %v", err)
	}
	if _, err := manager.InstallPlugin(context.Background(), source.Name, "nuclei"); err != nil {
		t.Fatalf("InstallPlugin first plugin: %v", err)
	}

	secondRepo := writeFixtureRepository(t)
	writeFile(t, filepath.Join(secondRepo, "repository.yaml"), `schema_version: 1
name: Second Fixture Plugins
description: fixture
plugins:
  - id: duplicate
    path: plugins/duplicate
`)
	duplicateDir := filepath.Join(secondRepo, "plugins", "duplicate")
	if err := os.RemoveAll(duplicateDir); err != nil {
		t.Fatalf("remove duplicate dir: %v", err)
	}
	if err := copyDir(filepath.Join(secondRepo, "plugins", "nuclei"), duplicateDir); err != nil {
		t.Fatalf("copy duplicate plugin: %v", err)
	}
	manifestPath := filepath.Join(duplicateDir, "plugin.yaml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read duplicate manifest: %v", err)
	}
	data = []byte(strings.Replace(string(data), "id: nuclei", "id: duplicate", 1))
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		t.Fatalf("write duplicate manifest: %v", err)
	}

	secondSource, err := manager.AddOrSyncSource(context.Background(), "second", secondRepo)
	if err != nil {
		t.Fatalf("AddOrSyncSource second: %v", err)
	}
	if _, err := manager.InstallPlugin(context.Background(), secondSource.Name, "duplicate"); err == nil {
		t.Fatal("expected installed plugin tool name conflict")
	} else if !strings.Contains(err.Error(), "tool name conflicts") {
		t.Fatalf("expected tool name conflict error, got %v", err)
	}
}
