package pluginstore

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

func (m *Manager) InstallPlugin(ctx context.Context, sourceName string, pluginID string) (InstalledPlugin, error) {
	if err := m.Ensure(); err != nil {
		return InstalledPlugin{}, err
	}
	if err := validateID(pluginID); err != nil {
		return InstalledPlugin{}, err
	}
	cat, err := m.CatalogForSource(sourceName)
	if err != nil {
		return InstalledPlugin{}, err
	}
	plugin, ok := cat.FindPlugin(pluginID)
	if !ok {
		return InstalledPlugin{}, fmt.Errorf("plugin %q not found in source %q", pluginID, sourceName)
	}
	toolNames, err := loadToolNames(plugin.Directory, plugin.Tools)
	if err != nil {
		return InstalledPlugin{}, err
	}
	if err := validateExposedTools(plugin, toolNames); err != nil {
		return InstalledPlugin{}, err
	}
	if conflicts := m.ToolNameConflicts(pluginID, toolNames); len(conflicts) > 0 {
		return InstalledPlugin{}, fmt.Errorf("plugin %q tool name conflicts with existing MCP tools: %s", plugin.ID, strings.Join(conflicts, ", "))
	}
	target := m.installedPluginDir(pluginID)
	if err := copyDir(plugin.Directory, target); err != nil {
		return InstalledPlugin{}, err
	}
	if err := installRuntime(ctx, target, plugin, m.githubToken); err != nil {
		_ = os.RemoveAll(target)
		return InstalledPlugin{}, err
	}
	now := time.Now()
	installed := InstalledPlugin{
		ID:           plugin.ID,
		Name:         plugin.Name,
		Version:      plugin.Version,
		SourceName:   sourceName,
		SourceURL:    plugin.SourceURL,
		PluginPath:   plugin.PluginRef.Path,
		InstalledDir: m.registryInstalledPath(pluginID),
		Enabled:      true,
		ToolNames:    toolNames,
		InstalledAt:  now,
		UpdatedAt:    now,
	}
	reg, err := m.LoadRegistry()
	if err != nil {
		return InstalledPlugin{}, err
	}
	upsertInstalled(&reg, installed)
	if err := m.SaveRegistry(reg); err != nil {
		return InstalledPlugin{}, err
	}
	return installed, nil
}

func (m *Manager) UninstallPlugin(pluginID string) (InstalledPlugin, error) {
	reg, err := m.LoadRegistry()
	if err != nil {
		return InstalledPlugin{}, err
	}
	out := reg.Installed[:0]
	var removed InstalledPlugin
	for _, installed := range reg.Installed {
		if installed.ID == pluginID {
			removed = installed
			continue
		}
		out = append(out, installed)
	}
	if removed.ID == "" {
		return InstalledPlugin{}, fmt.Errorf("installed plugin %q not found", pluginID)
	}
	reg.Installed = out
	if err := m.SaveRegistry(reg); err != nil {
		return InstalledPlugin{}, err
	}
	return removed, nil
}

func validateExposedTools(plugin PluginManifest, toolNames []string) error {
	if len(plugin.MCP.ExposeTools) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(toolNames))
	for _, name := range toolNames {
		if name != "" {
			seen[name] = struct{}{}
		}
	}
	var missing []string
	for _, name := range plugin.MCP.ExposeTools {
		if _, ok := seen[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("plugin %q mcp.expose_tools not found in tool yaml names: %s", plugin.ID, strings.Join(missing, ", "))
	}
	return nil
}
