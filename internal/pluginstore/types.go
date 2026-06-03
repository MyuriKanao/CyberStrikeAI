package pluginstore

import "time"

const SchemaVersion = 1

type RepositoryManifest struct {
	SchemaVersion int         `yaml:"schema_version" json:"schema_version"`
	Name          string      `yaml:"name" json:"name"`
	Description   string      `yaml:"description,omitempty" json:"description,omitempty"`
	Plugins       []PluginRef `yaml:"plugins" json:"plugins"`
}

type PluginRef struct {
	ID   string `yaml:"id" json:"id"`
	Path string `yaml:"path" json:"path"`
}

type PluginManifest struct {
	SchemaVersion int            `yaml:"schema_version" json:"schema_version"`
	ID            string         `yaml:"id" json:"id"`
	Name          string         `yaml:"name" json:"name"`
	Version       string         `yaml:"version" json:"version"`
	Description   string         `yaml:"description" json:"description"`
	Categories    []string       `yaml:"categories,omitempty" json:"categories,omitempty"`
	Tags          []string       `yaml:"tags,omitempty" json:"tags,omitempty"`
	Platforms     []Platform     `yaml:"platforms,omitempty" json:"platforms,omitempty"`
	Tools         []ToolRef      `yaml:"tools,omitempty" json:"tools,omitempty"`
	Runtime       Runtime        `yaml:"runtime,omitempty" json:"runtime,omitempty"`
	Permissions   Permissions    `yaml:"permissions,omitempty" json:"permissions,omitempty"`
	MCP           MCPDeclaration `yaml:"mcp,omitempty" json:"mcp,omitempty"`
	LLM           LLMGuidance    `yaml:"llm,omitempty" json:"llm,omitempty"`
	Directory     string         `yaml:"-" json:"directory,omitempty"`
	SourceName    string         `yaml:"-" json:"source_name,omitempty"`
	SourceURL     string         `yaml:"-" json:"source_url,omitempty"`
	PluginRef     PluginRef      `yaml:"-" json:"plugin_ref,omitempty"`
}

type Platform struct {
	OS   string `yaml:"os" json:"os"`
	Arch string `yaml:"arch" json:"arch"`
}

type ToolRef struct {
	Type string `yaml:"type" json:"type"`
	File string `yaml:"file" json:"file"`
}

type Runtime struct {
	Install RuntimeInstall `yaml:"install,omitempty" json:"install,omitempty"`
	Verify  RuntimeVerify  `yaml:"verify,omitempty" json:"verify,omitempty"`
	Paths   RuntimePaths   `yaml:"paths,omitempty" json:"paths,omitempty"`
}

type RuntimeInstall struct {
	Type       string   `yaml:"type,omitempty" json:"type,omitempty"`
	Repo       string   `yaml:"repo,omitempty" json:"repo,omitempty"`
	Version    string   `yaml:"version,omitempty" json:"version,omitempty"`
	Package    string   `yaml:"package,omitempty" json:"package,omitempty"`
	Packages   []string `yaml:"packages,omitempty" json:"packages,omitempty"`
	Asset      string   `yaml:"asset,omitempty" json:"asset,omitempty"`
	BinaryName string   `yaml:"binary_name,omitempty" json:"binary_name,omitempty"`
	SHA256     string   `yaml:"sha256,omitempty" json:"sha256,omitempty"`
}

type RuntimeVerify struct {
	Command string   `yaml:"command,omitempty" json:"command,omitempty"`
	Args    []string `yaml:"args,omitempty" json:"args,omitempty"`
}

type RuntimePaths struct {
	Bin       string `yaml:"bin,omitempty" json:"bin,omitempty"`
	Data      string `yaml:"data,omitempty" json:"data,omitempty"`
	Templates string `yaml:"templates,omitempty" json:"templates,omitempty"`
}

type Permissions struct {
	Network    bool `yaml:"network,omitempty" json:"network,omitempty"`
	Execute    bool `yaml:"execute,omitempty" json:"execute,omitempty"`
	Filesystem bool `yaml:"filesystem,omitempty" json:"filesystem,omitempty"`
}

type MCPDeclaration struct {
	ExposeTools          []string `yaml:"expose_tools,omitempty" json:"expose_tools,omitempty"`
	DefaultEnabled       bool     `yaml:"default_enabled,omitempty" json:"default_enabled,omitempty"`
	DefaultAlwaysVisible bool     `yaml:"default_always_visible,omitempty" json:"default_always_visible,omitempty"`
}

type LLMGuidance struct {
	UseWhen     []string `yaml:"use_when,omitempty" json:"use_when,omitempty"`
	InstallHint string   `yaml:"install_hint,omitempty" json:"install_hint,omitempty"`
	Limitations []string `yaml:"limitations,omitempty" json:"limitations,omitempty"`
}

type Catalog struct {
	Repository RepositoryManifest `json:"repository"`
	Plugins    []PluginManifest   `json:"plugins"`
}

type Source struct {
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Path      string    `json:"path"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Registry struct {
	Sources   []Source          `json:"sources"`
	Installed []InstalledPlugin `json:"installed"`
}

type InstalledPlugin struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Version      string    `json:"version"`
	SourceName   string    `json:"source_name"`
	SourceURL    string    `json:"source_url"`
	PluginPath   string    `json:"plugin_path"`
	InstalledDir string    `json:"installed_dir"`
	Enabled      bool      `json:"enabled"`
	ToolNames    []string  `json:"tool_names,omitempty"`
	InstalledAt  time.Time `json:"installed_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
