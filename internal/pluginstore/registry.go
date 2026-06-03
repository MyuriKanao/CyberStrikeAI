package pluginstore

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type reservedTool struct {
	Command string
	Strict  bool
}

type Manager struct {
	rootDir       string
	sourcesDir    string
	installedDir  string
	registryPath  string
	githubToken   string
	reservedTools map[string]reservedTool
}

func New(rootDir string) *Manager {
	if rootDir == "" {
		rootDir = filepath.Join("data", "plugins")
	}
	return &Manager{
		rootDir:      rootDir,
		sourcesDir:   filepath.Join(rootDir, "sources"),
		installedDir: filepath.Join(rootDir, "installed"),
		registryPath: filepath.Join(rootDir, "registry.json"),
	}
}

func (m *Manager) RootDir() string {
	if m == nil {
		return ""
	}
	return m.rootDir
}

func (m *Manager) SetGitHubToken(token string) {
	if m == nil {
		return
	}
	m.githubToken = strings.TrimSpace(token)
}

func (m *Manager) SetReservedToolNames(names []string) {
	if m == nil {
		return
	}
	m.reservedTools = make(map[string]reservedTool, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name != "" {
			m.reservedTools[name] = reservedTool{Strict: true}
		}
	}
}

func (m *Manager) SetReservedToolCommands(commands map[string]string) {
	if m == nil {
		return
	}
	m.reservedTools = make(map[string]reservedTool, len(commands))
	for name, command := range commands {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		m.reservedTools[name] = reservedTool{Command: strings.TrimSpace(command)}
	}
}

func (m *Manager) sourceDir(name string) string {
	return filepath.Join(m.sourcesDir, name)
}

func (m *Manager) installedPluginDir(pluginID string) string {
	return filepath.Join(m.installedDir, pluginID)
}

func (m *Manager) registrySourcePath(name string) string {
	return filepath.ToSlash(filepath.Join("sources", name))
}

func (m *Manager) registryInstalledPath(pluginID string) string {
	return filepath.ToSlash(filepath.Join("installed", pluginID))
}

func (m *Manager) resolveRegistryPath(path string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(m.rootDir, filepath.FromSlash(path))
}

func (m *Manager) RuntimeBinDirs() []string {
	if m == nil {
		return nil
	}
	reg, _ := m.LoadRegistry()
	out := []string{
		filepath.Join(m.rootDir, "runtime", "bin"),
		filepath.Join(m.rootDir, "tool-runtime", "bin"),
	}
	for _, installed := range reg.Installed {
		if installed.Enabled {
			installedDir := m.resolveRegistryPath(installed.InstalledDir)
			out = append(out, filepath.Join(installedDir, "runtime", "bin"))
			if manifest, err := loadPluginManifest(installedDir); err == nil {
				if binDir, err := runtimeBinDir(installedDir, manifest); err == nil {
					out = append(out, binDir)
				}
			}
		}
	}
	return normalizePathDirs(out)
}

func (m *Manager) Ensure() error {
	if m == nil {
		return errors.New("plugin manager is nil")
	}
	for _, dir := range []string{m.rootDir, m.sourcesDir, m.installedDir, filepath.Join(m.rootDir, "runtime", "bin")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if _, err := os.Stat(m.registryPath); errors.Is(err, os.ErrNotExist) {
		return m.SaveRegistry(Registry{})
	}
	return nil
}

func (m *Manager) LoadRegistry() (Registry, error) {
	var reg Registry
	data, err := os.ReadFile(m.registryPath)
	if errors.Is(err, os.ErrNotExist) {
		return reg, nil
	}
	if err != nil {
		return reg, err
	}
	if len(data) == 0 {
		return reg, nil
	}
	if err := json.Unmarshal(data, &reg); err != nil {
		return reg, err
	}
	sanitizeRegistrySourceURLs(&reg)
	return reg, nil
}

func (m *Manager) SaveRegistry(reg Registry) error {
	if err := os.MkdirAll(filepath.Dir(m.registryPath), 0o755); err != nil {
		return err
	}
	sanitizeRegistrySourceURLs(&reg)
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	tmp := m.registryPath + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, m.registryPath)
}

func (m *Manager) ListSources() ([]Source, error) {
	reg, err := m.LoadRegistry()
	if err != nil {
		return nil, err
	}
	return append([]Source(nil), reg.Sources...), nil
}

func (m *Manager) ToolNameConflicts(pluginID string, toolNames []string) []string {
	if m == nil {
		return nil
	}
	conflicts := map[string]struct{}{}
	for _, name := range toolNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if reserved, ok := m.reservedTools[name]; ok && reserved.conflicts() {
			conflicts[name] = struct{}{}
		}
	}
	reg, err := m.LoadRegistry()
	if err != nil {
		return sortedKeys(conflicts)
	}
	for _, installed := range reg.Installed {
		if installed.ID == pluginID || !installed.Enabled {
			continue
		}
		for _, existing := range installed.ToolNames {
			existing = strings.TrimSpace(existing)
			if existing == "" {
				continue
			}
			for _, name := range toolNames {
				if strings.TrimSpace(name) == existing {
					conflicts[existing] = struct{}{}
				}
			}
		}
	}
	return sortedKeys(conflicts)
}

func (r reservedTool) conflicts() bool {
	if r.Strict {
		return true
	}
	command := strings.TrimSpace(r.Command)
	if command == "" {
		return true
	}
	if strings.HasPrefix(command, "internal:") {
		return true
	}
	if filepath.IsAbs(command) || strings.ContainsAny(command, `/\`) {
		info, err := os.Stat(command)
		return err == nil && !info.IsDir()
	}
	_, err := exec.LookPath(command)
	return err == nil
}

func sortedKeys(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (m *Manager) ListInstalled() ([]InstalledPlugin, error) {
	reg, err := m.LoadRegistry()
	if err != nil {
		return nil, err
	}
	return append([]InstalledPlugin(nil), reg.Installed...), nil
}

func upsertSource(reg *Registry, src Source) {
	for i := range reg.Sources {
		if reg.Sources[i].Name == src.Name {
			reg.Sources[i] = src
			return
		}
	}
	reg.Sources = append(reg.Sources, src)
}

func upsertInstalled(reg *Registry, installed InstalledPlugin) {
	now := time.Now()
	for i := range reg.Installed {
		if reg.Installed[i].ID == installed.ID {
			if installed.InstalledAt.IsZero() {
				installed.InstalledAt = reg.Installed[i].InstalledAt
			}
			if installed.InstalledAt.IsZero() {
				installed.InstalledAt = now
			}
			installed.UpdatedAt = now
			reg.Installed[i] = installed
			return
		}
	}
	if installed.InstalledAt.IsZero() {
		installed.InstalledAt = now
	}
	if installed.UpdatedAt.IsZero() {
		installed.UpdatedAt = now
	}
	reg.Installed = append(reg.Installed, installed)
}
