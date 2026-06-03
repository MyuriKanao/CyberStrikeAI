package config

import (
	"path/filepath"
	"testing"
)

func TestLoadToolsFromDirIncludesWebSearch(t *testing.T) {
	tools, err := LoadToolsFromDir(filepath.Join("..", "..", "tools"))
	if err != nil {
		t.Fatalf("LoadToolsFromDir() error = %v", err)
	}

	for _, tool := range tools {
		if tool.Name != "web_search" {
			continue
		}
		if !tool.Enabled {
			t.Fatal("web_search tool should be enabled")
		}
		for _, param := range tool.Parameters {
			if param.Name == "query" && param.Required {
				return
			}
		}
		t.Fatal("web_search tool should require query")
	}

	t.Fatal("web_search tool was not loaded from tools directory")
}
