package pluginstore

import (
	"fmt"
	"path/filepath"

	"cyberstrike-ai/internal/config"
)

func (m *Manager) LoadInstalledTools() ([]config.ToolConfig, error) {
	reg, err := m.LoadRegistry()
	if err != nil {
		return nil, err
	}
	var out []config.ToolConfig
	for _, installed := range reg.Installed {
		if !installed.Enabled {
			continue
		}
		installedDir := m.resolveRegistryPath(installed.InstalledDir)
		manifest, err := loadPluginManifest(installedDir)
		if err != nil {
			return nil, fmt.Errorf("installed plugin %q: %w", installed.ID, err)
		}
		tools, err := loadTools(installedDir, manifest.Tools)
		if err != nil {
			return nil, fmt.Errorf("installed plugin %q tools: %w", installed.ID, err)
		}
		out = append(out, tools...)
	}
	return out, nil
}

func MergeTools(base []config.ToolConfig, pluginTools []config.ToolConfig) ([]config.ToolConfig, []string) {
	merged := append([]config.ToolConfig(nil), base...)
	seen := make(map[string]struct{}, len(base)+len(pluginTools))
	for _, tool := range base {
		if tool.Name != "" {
			seen[tool.Name] = struct{}{}
		}
	}
	var skipped []string
	for _, tool := range pluginTools {
		if _, ok := seen[tool.Name]; ok {
			skipped = append(skipped, tool.Name)
			continue
		}
		seen[tool.Name] = struct{}{}
		merged = append(merged, tool)
	}
	return merged, skipped
}

func loadToolNames(pluginDir string, refs []ToolRef) ([]string, error) {
	tools, err := loadTools(pluginDir, refs)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names, nil
}

func loadTools(pluginDir string, refs []ToolRef) ([]config.ToolConfig, error) {
	var out []config.ToolConfig
	for _, ref := range refs {
		if ref.Type != "command" {
			continue
		}
		path, err := safeJoin(pluginDir, filepath.Clean(ref.File))
		if err != nil {
			return nil, err
		}
		tool, err := config.LoadToolFromFile(path)
		if err != nil {
			return nil, err
		}
		out = append(out, *tool)
	}
	return out, nil
}
