package security

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"cyberstrike-ai/internal/config"
	"cyberstrike-ai/internal/mcp"

	"go.uber.org/zap"
)

func TestExecutorFindsToolInExtraPathWhenProcessPathIsEmpty(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is Unix-specific")
	}

	binDir := t.TempDir()
	toolPath := filepath.Join(binDir, "path-fixture")
	if err := os.WriteFile(toolPath, []byte("#!/bin/sh\nprintf '%s\\n' \"$PATH\"\n"), 0o755); err != nil {
		t.Fatalf("write fixture tool: %v", err)
	}

	t.Setenv("PATH", "")

	cfg := &config.SecurityConfig{
		ExtraPathDirs: []string{binDir},
		Tools: []config.ToolConfig{
			{
				Name:    "path-fixture",
				Command: "path-fixture",
				Args:    []string{"run"},
				Enabled: true,
			},
		},
	}
	executor := NewExecutor(cfg, mcp.NewServer(zap.NewNop()), zap.NewNop())

	res, err := executor.ExecuteTool(context.Background(), "path-fixture", map[string]interface{}{})
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatalf("expected tool success, got %+v", res)
	}
	if got := res.Content[0].Text; !strings.Contains(got, binDir) {
		t.Fatalf("child PATH %q does not contain extra path dir %q", got, binDir)
	}
}
