package pluginstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestExternalPluginRepositoryInstall(t *testing.T) {
	repo := os.Getenv("CYBERSTRIKE_PLUGIN_REPO")
	pluginID := os.Getenv("CYBERSTRIKE_PLUGIN_ID")
	if repo == "" || pluginID == "" {
		t.Skip("set CYBERSTRIKE_PLUGIN_REPO and CYBERSTRIKE_PLUGIN_ID to run external plugin install verification")
	}

	manager := New(filepath.Join(t.TempDir(), "plugins"))
	source, err := manager.AddOrSyncSource(context.Background(), "external", repo)
	if err != nil {
		t.Fatalf("AddOrSyncSource: %v", err)
	}
	installed, err := manager.InstallPlugin(context.Background(), source.Name, pluginID)
	if err != nil {
		t.Fatalf("InstallPlugin: %v", err)
	}
	if filepath.IsAbs(installed.InstalledDir) {
		t.Fatalf("installed dir should be registry-relative, got %q", installed.InstalledDir)
	}
	if len(installed.ToolNames) == 0 {
		t.Fatalf("expected installed tool names, got %+v", installed)
	}

	tools, err := manager.LoadInstalledTools()
	if err != nil {
		t.Fatalf("LoadInstalledTools: %v", err)
	}
	for _, tool := range tools {
		if tool.Name == pluginID {
			return
		}
	}
	t.Fatalf("installed tools did not include %q: %+v", pluginID, tools)
}
