package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"cyberstrike-ai/internal/config"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/schema"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

func TestHTTPDocumentRerankerReranksByProviderResponse(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotReq map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"results": [
				{"index": 1, "relevance_score": 0.91},
				{"index": 0, "relevance_score": 0.42}
			]
		}`))
	}))
	defer srv.Close()

	rr, err := NewHTTPDocumentReranker(config.RerankConfig{
		Enabled: true,
		Model:   "bge-reranker-v2-m3",
		BaseURL: srv.URL + "/v1",
		APIKey:  "test-key",
		TopN:    2,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewHTTPDocumentReranker: %v", err)
	}

	docs := []*schema.Document{
		{ID: "a", Content: "alpha"},
		{ID: "b", Content: "bravo"},
	}
	docs[0].WithScore(0.6)
	docs[1].WithScore(0.5)

	out, err := rr.Rerank(context.Background(), "query", docs)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}

	if gotPath != "/v1/rerank" {
		t.Fatalf("path=%q want /v1/rerank", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("auth=%q", gotAuth)
	}
	if gotReq["model"] != "bge-reranker-v2-m3" || gotReq["query"] != "query" {
		t.Fatalf("bad request: %#v", gotReq)
	}
	if gotReq["top_n"].(float64) != 2 {
		t.Fatalf("top_n=%#v", gotReq["top_n"])
	}
	if len(out) != 2 || out[0].ID != "b" || out[1].ID != "a" {
		t.Fatalf("order=%v", []string{out[0].ID, out[1].ID})
	}
	if out[0].Score() != 0.91 {
		t.Fatalf("score=%v want 0.91", out[0].Score())
	}
	if out[0].MetaData["rerank_score"] != 0.91 {
		t.Fatalf("metadata rerank_score=%#v", out[0].MetaData["rerank_score"])
	}
	if out[0].MetaData["vector_score"] != 0.5 {
		t.Fatalf("metadata vector_score=%#v", out[0].MetaData["vector_score"])
	}
}

func TestRetrieverUpdateConfigInstallsAndClearsReranker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"index":0,"relevance_score":0.8}]}`))
	}))
	defer srv.Close()

	r := NewRetriever(nil, nil, &RetrievalConfig{TopK: 5}, zap.NewNop())
	if r.documentReranker() != nil {
		t.Fatal("reranker should be nil by default")
	}

	r.UpdateConfig(&RetrievalConfig{
		TopK: 5,
		Rerank: config.RerankConfig{
			Enabled: true,
			Model:   "rerank-model",
			BaseURL: srv.URL,
		},
	})
	if r.documentReranker() == nil {
		t.Fatal("expected configured reranker")
	}

	r.UpdateConfig(&RetrievalConfig{
		TopK: 5,
		Rerank: config.RerankConfig{
			Enabled: false,
			Model:   "rerank-model",
			BaseURL: srv.URL,
		},
	})
	if r.documentReranker() != nil {
		t.Fatal("reranker should be cleared when disabled")
	}
}

func TestEffectiveRerankConfigUsesOpenAIFallback(t *testing.T) {
	got := EffectiveRerankConfig(config.RerankConfig{
		Enabled: true,
		Model:   "rerank-model",
	}, &config.OpenAIConfig{
		BaseURL: "https://example.test/v1",
		APIKey:  "fallback-key",
	})

	if got.Provider != "openai_compatible" {
		t.Fatalf("provider=%q", got.Provider)
	}
	if got.BaseURL != "https://example.test/v1" {
		t.Fatalf("base_url=%q", got.BaseURL)
	}
	if got.APIKey != "fallback-key" {
		t.Fatalf("api_key=%q", got.APIKey)
	}
}

func TestVectorEinoRetrieverFallsBackToVectorOrderWhenRerankFails(t *testing.T) {
	db := newTestKnowledgeDB(t)
	insertTestKnowledgeRow(t, db, "chunk-1", "item-1", 0, "best vector match", `[1,0]`)
	insertTestKnowledgeRow(t, db, "chunk-2", "item-2", 0, "second vector match", `[0.8,0.2]`)

	emb := &Embedder{
		eino: testEinoEmbedder{vec: []float64{1, 0}},
		config: &config.KnowledgeConfig{
			Embedding: config.EmbeddingConfig{Model: "test-embedding"},
		},
		maxRetries: 1,
	}
	r := NewRetriever(db, emb, &RetrievalConfig{TopK: 2, SimilarityThreshold: 0.1}, zap.NewNop())
	r.SetDocumentReranker(failingDocumentReranker{})

	docs, err := NewVectorEinoRetriever(r).Retrieve(context.Background(), "query")
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("len(docs)=%d want 2", len(docs))
	}
	if docs[0].ID != "chunk-1" || docs[1].ID != "chunk-2" {
		t.Fatalf("order=%v want [chunk-1 chunk-2]", []string{docs[0].ID, docs[1].ID})
	}
	if _, ok := docs[0].MetaData["rerank_score"]; ok {
		t.Fatalf("rerank metadata should not be attached on fallback: %#v", docs[0].MetaData)
	}
}

type testEinoEmbedder struct {
	vec []float64
}

func (e testEinoEmbedder) EmbedStrings(context.Context, []string, ...embedding.Option) ([][]float64, error) {
	return [][]float64{e.vec}, nil
}

type failingDocumentReranker struct{}

func (failingDocumentReranker) Rerank(context.Context, string, []*schema.Document) ([]*schema.Document, error) {
	return nil, errors.New("rerank unavailable")
}

func newTestKnowledgeDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	stmts := []string{
		`CREATE TABLE knowledge_base_items (
			id TEXT PRIMARY KEY,
			category TEXT NOT NULL,
			title TEXT NOT NULL,
			file_path TEXT NOT NULL,
			content TEXT,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);`,
		`CREATE TABLE knowledge_embeddings (
			id TEXT PRIMARY KEY,
			item_id TEXT NOT NULL,
			chunk_index INTEGER NOT NULL,
			chunk_text TEXT NOT NULL,
			embedding TEXT NOT NULL,
			sub_indexes TEXT NOT NULL DEFAULT '',
			embedding_model TEXT NOT NULL DEFAULT '',
			embedding_dim INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL
		);`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create test schema: %v", err)
		}
	}
	return db
}

func insertTestKnowledgeRow(t *testing.T, db *sql.DB, chunkID, itemID string, chunkIndex int, text, embeddingJSON string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO knowledge_base_items (id, category, title, file_path, content, created_at, updated_at)
		VALUES (?, 'SQL Injection', ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		itemID, "title-"+itemID, itemID+".md", text,
	); err != nil {
		t.Fatalf("insert item: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO knowledge_embeddings (id, item_id, chunk_index, chunk_text, embedding, embedding_model, embedding_dim, created_at)
		VALUES (?, ?, ?, ?, ?, 'test-embedding', 2, CURRENT_TIMESTAMP)`,
		chunkID, itemID, chunkIndex, text, embeddingJSON,
	); err != nil {
		t.Fatalf("insert embedding: %v", err)
	}
}
