package pluginstore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCatalog(t *testing.T) {
	repo := writeFixtureRepository(t)

	catalog, err := LoadCatalog(repo)
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	if catalog.Repository.Name != "Fixture Plugins" {
		t.Fatalf("repository name = %q", catalog.Repository.Name)
	}
	if len(catalog.Plugins) != 1 {
		t.Fatalf("plugin count = %d", len(catalog.Plugins))
	}
	plugin := catalog.Plugins[0]
	if plugin.ID != "nuclei" {
		t.Fatalf("plugin id = %q", plugin.ID)
	}
	if len(plugin.Tools) != 1 || plugin.Tools[0].File != "tools/nuclei.yaml" {
		t.Fatalf("unexpected plugin tools: %+v", plugin.Tools)
	}
}

func writeFixtureRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "repository.yaml"), `schema_version: 1
name: Fixture Plugins
description: fixture
plugins:
  - id: nuclei
    path: plugins/nuclei
`)
	writeFile(t, filepath.Join(root, "plugins", "nuclei", "plugin.yaml"), `schema_version: 1
id: nuclei
name: Nuclei
version: 1.0.0
description: scanner
categories: [scanner, web]
platforms:
  - os: linux
    arch: amd64
tools:
  - type: command
    file: tools/nuclei.yaml
runtime:
  install:
    type: none
permissions:
  network: true
  execute: true
mcp:
  expose_tools: [nuclei]
llm:
  use_when:
    - scan known vulnerabilities
`)
	writeFile(t, filepath.Join(root, "plugins", "nuclei", "tools", "nuclei.yaml"), `name: nuclei
command: nuclei
enabled: true
description: nuclei scanner
parameters:
  - name: target
    type: string
    description: target
    required: true
`)
	return root
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
