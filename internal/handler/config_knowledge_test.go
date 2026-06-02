package handler

import (
	"testing"

	"cyberstrike-ai/internal/config"

	"gopkg.in/yaml.v3"
)

func TestUpdateKnowledgeConfigWritesRerankConfig(t *testing.T) {
	doc := &yaml.Node{
		Kind: yaml.DocumentNode,
		Content: []*yaml.Node{{
			Kind: yaml.MappingNode,
		}},
	}

	updateKnowledgeConfig(doc, config.KnowledgeConfig{
		Enabled:  true,
		BasePath: "knowledge_base",
		Embedding: config.EmbeddingConfig{
			Provider: "openai",
			Model:    "text-embedding-3-small",
		},
		Retrieval: config.RetrievalConfig{
			TopK:                5,
			SimilarityThreshold: 0.65,
			Rerank: config.RerankConfig{
				Enabled:               true,
				Provider:              "openai_compatible",
				Model:                 "bge-reranker-v2-m3",
				BaseURL:               "https://api.example.test/v1",
				APIKey:                "test-key",
				TopN:                  12,
				RequestTimeoutSeconds: 45,
			},
		},
	})

	var got map[string]any
	if err := doc.Decode(&got); err != nil {
		t.Fatalf("decode yaml: %v", err)
	}

	knowledgeMap := got["knowledge"].(map[string]any)
	retrievalMap := knowledgeMap["retrieval"].(map[string]any)
	rerankMap := retrievalMap["rerank"].(map[string]any)

	if rerankMap["enabled"] != true {
		t.Fatalf("enabled=%#v", rerankMap["enabled"])
	}
	if rerankMap["provider"] != "openai_compatible" {
		t.Fatalf("provider=%#v", rerankMap["provider"])
	}
	if rerankMap["model"] != "bge-reranker-v2-m3" {
		t.Fatalf("model=%#v", rerankMap["model"])
	}
	if rerankMap["base_url"] != "https://api.example.test/v1" {
		t.Fatalf("base_url=%#v", rerankMap["base_url"])
	}
	if rerankMap["api_key"] != "test-key" {
		t.Fatalf("api_key=%#v", rerankMap["api_key"])
	}
	if rerankMap["top_n"] != 12 {
		t.Fatalf("top_n=%#v", rerankMap["top_n"])
	}
	if rerankMap["request_timeout_seconds"] != 45 {
		t.Fatalf("request_timeout_seconds=%#v", rerankMap["request_timeout_seconds"])
	}
}
