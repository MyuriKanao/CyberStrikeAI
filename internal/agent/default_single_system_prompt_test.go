package agent

import (
	"strings"
	"testing"
)

func TestDefaultSingleAgentSystemPromptRequiresNamedVulnerabilityResearch(t *testing.T) {
	prompt := DefaultSingleAgentSystemPrompt()
	for _, want := range []string{
		"CVE",
		"CNVD",
		"search_knowledge_base",
		"web_search",
		"公开资料",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("DefaultSingleAgentSystemPrompt() missing %q", want)
		}
	}
}
