package multiagent

import (
	"strings"
	"testing"
)

func TestDefaultPlanExecuteInstructionRequiresNamedVulnerabilityResearch(t *testing.T) {
	prompt := DefaultPlanExecuteOrchestratorInstruction()
	for _, want := range []string{
		"CVE",
		"CNVD",
		"search_knowledge_base",
		"web_search",
		"公开资料",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("DefaultPlanExecuteOrchestratorInstruction() missing %q", want)
		}
	}
}
