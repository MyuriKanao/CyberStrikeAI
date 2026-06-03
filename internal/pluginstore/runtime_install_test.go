package pluginstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyFileSHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "asset")
	if err := os.WriteFile(path, []byte("ffuf\n"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	const good = "0b86e63659834d713a7b6e4b0c7d1b6d83c1c0c69b533bcabfea867305647a5f"
	if err := verifyFileSHA256(path, good); err != nil {
		t.Fatalf("verify good checksum: %v", err)
	}
	if err := verifyFileSHA256(path, "sha256:"+good); err != nil {
		t.Fatalf("verify prefixed checksum: %v", err)
	}
	if err := verifyFileSHA256(path, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err == nil {
		t.Fatal("expected checksum mismatch")
	}
}

func TestSelectReleaseAssetMatchesVersionedFFufAsset(t *testing.T) {
	asset := selectReleaseAsset([]githubReleaseAsset{
		{Name: "ffuf_2.1.0_linux_386.tar.gz", BrowserDownloadURL: "https://example.test/386"},
		{Name: "ffuf_2.1.0_linux_amd64.tar.gz", BrowserDownloadURL: "https://example.test/amd64"},
	}, "ffuf_linux_amd64.tar.gz")

	if asset.Name != "ffuf_2.1.0_linux_amd64.tar.gz" {
		t.Fatalf("selected asset = %+v", asset)
	}
}

func TestPythonInstallSpecs(t *testing.T) {
	specs, err := pythonInstallSpecs(RuntimeInstall{
		Package:  "arjun",
		Version:  "2.2.7",
		Packages: []string{"setuptools<81"},
	})
	if err != nil {
		t.Fatalf("pythonInstallSpecs: %v", err)
	}
	want := []string{"setuptools<81", "arjun==2.2.7"}
	if len(specs) != len(want) {
		t.Fatalf("spec count = %d, want %d: %+v", len(specs), len(want), specs)
	}
	for i := range want {
		if specs[i] != want[i] {
			t.Fatalf("spec[%d] = %q, want %q", i, specs[i], want[i])
		}
	}
}

func TestGitHubTokenAuth(t *testing.T) {
	const token = "ghp_example"

	args := gitAuthArgs(token, "https://github.com/example/CyberStrikeAI-Plugins.git")
	if len(args) != 2 {
		t.Fatalf("git auth args count = %d, want 2: %+v", len(args), args)
	}
	if args[0] != "-c" {
		t.Fatalf("first git auth arg = %q, want -c", args[0])
	}
	if args[1] != "http.https://github.com/.extraHeader=Authorization: Bearer "+token {
		t.Fatalf("git auth header arg = %q", args[1])
	}
	if strings.Contains(strings.Join(args, " "), "github.com/example/CyberStrikeAI-Plugins.git@") {
		t.Fatalf("git auth args should not inject token into the repository URL: %+v", args)
	}
	if got := gitAuthArgs(token, "https://gitlab.com/example/repo.git"); got != nil {
		t.Fatalf("non-github sources should not receive github token args: %+v", got)
	}
	if got := githubAuthorizationHeader(token); got != "Bearer "+token {
		t.Fatalf("github authorization header = %q", got)
	}
	if got := githubAuthorizationHeader(" "); got != "" {
		t.Fatalf("empty token should not produce auth header, got %q", got)
	}
}

func TestGitHubReleaseBinaryName(t *testing.T) {
	got, err := githubReleaseBinaryName(RuntimeInstall{BinaryName: "fscan"}, "fscan_2.1.3_linux_x64")
	if err != nil {
		t.Fatalf("githubReleaseBinaryName: %v", err)
	}
	if got != "fscan" {
		t.Fatalf("binary name = %q, want fscan", got)
	}

	got, err = githubReleaseBinaryName(RuntimeInstall{}, "nuclei_linux_amd64.zip")
	if err != nil {
		t.Fatalf("githubReleaseBinaryName without override: %v", err)
	}
	if got != "nuclei_linux_amd64.zip" {
		t.Fatalf("binary name fallback = %q", got)
	}

	if _, err := githubReleaseBinaryName(RuntimeInstall{BinaryName: "../fscan"}, "asset"); err == nil {
		t.Fatal("expected unsafe binary name to be rejected")
	}
}
