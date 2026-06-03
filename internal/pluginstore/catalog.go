package pluginstore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func LoadCatalog(repoDir string) (*Catalog, error) {
	manifestPath := filepath.Join(repoDir, "repository.yaml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read repository manifest: %w", err)
	}

	var repo RepositoryManifest
	if err := yaml.Unmarshal(data, &repo); err != nil {
		return nil, fmt.Errorf("parse repository manifest: %w", err)
	}
	if repo.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("unsupported repository schema_version %d", repo.SchemaVersion)
	}
	if strings.TrimSpace(repo.Name) == "" {
		return nil, fmt.Errorf("repository name is required")
	}

	catalog := &Catalog{Repository: repo}
	for _, ref := range repo.Plugins {
		if err := validateID(ref.ID); err != nil {
			return nil, err
		}
		pluginDir, err := safeJoin(repoDir, ref.Path)
		if err != nil {
			return nil, fmt.Errorf("plugin %q path: %w", ref.ID, err)
		}
		plugin, err := loadPluginManifest(pluginDir)
		if err != nil {
			return nil, fmt.Errorf("plugin %q: %w", ref.ID, err)
		}
		if plugin.ID != ref.ID {
			return nil, fmt.Errorf("plugin ref id %q does not match manifest id %q", ref.ID, plugin.ID)
		}
		plugin.Directory = pluginDir
		plugin.PluginRef = ref
		catalog.Plugins = append(catalog.Plugins, plugin)
	}
	return catalog, nil
}

func loadPluginManifest(pluginDir string) (PluginManifest, error) {
	data, err := os.ReadFile(filepath.Join(pluginDir, "plugin.yaml"))
	if err != nil {
		return PluginManifest{}, fmt.Errorf("read plugin manifest: %w", err)
	}
	var plugin PluginManifest
	if err := yaml.Unmarshal(data, &plugin); err != nil {
		return PluginManifest{}, fmt.Errorf("parse plugin manifest: %w", err)
	}
	if plugin.SchemaVersion != SchemaVersion {
		return PluginManifest{}, fmt.Errorf("unsupported plugin schema_version %d", plugin.SchemaVersion)
	}
	if err := validateID(plugin.ID); err != nil {
		return PluginManifest{}, err
	}
	if strings.TrimSpace(plugin.Name) == "" {
		return PluginManifest{}, fmt.Errorf("plugin name is required")
	}
	if strings.TrimSpace(plugin.Version) == "" {
		return PluginManifest{}, fmt.Errorf("plugin version is required")
	}
	for _, tool := range plugin.Tools {
		if tool.Type == "" || tool.File == "" {
			return PluginManifest{}, fmt.Errorf("tool type and file are required")
		}
		if _, err := safeJoin(pluginDir, tool.File); err != nil {
			return PluginManifest{}, fmt.Errorf("tool file %q: %w", tool.File, err)
		}
	}
	return plugin, nil
}

func (c *Catalog) FindPlugin(id string) (PluginManifest, bool) {
	if c == nil {
		return PluginManifest{}, false
	}
	for _, plugin := range c.Plugins {
		if plugin.ID == id {
			return plugin, true
		}
	}
	return PluginManifest{}, false
}
