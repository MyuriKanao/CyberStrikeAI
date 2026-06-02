package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cyberstrike-ai/internal/config"

	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

const defaultRerankTimeout = 60 * time.Second

// EffectiveRerankConfig returns a runtime rerank config with OpenAI-compatible defaults filled in.
func EffectiveRerankConfig(cfg config.RerankConfig, openAIConfig *config.OpenAIConfig) config.RerankConfig {
	out := cfg
	if !out.Enabled {
		return out
	}
	if strings.TrimSpace(out.Provider) == "" {
		out.Provider = "openai_compatible"
	}
	if openAIConfig != nil {
		if strings.TrimSpace(out.BaseURL) == "" {
			out.BaseURL = openAIConfig.BaseURL
		}
		if strings.TrimSpace(out.APIKey) == "" {
			out.APIKey = openAIConfig.APIKey
		}
	}
	return out
}

// NewHTTPDocumentReranker creates a generic HTTP reranker for providers exposing a /rerank endpoint.
func NewHTTPDocumentReranker(cfg config.RerankConfig, logger *zap.Logger) (DocumentReranker, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		return nil, fmt.Errorf("rerank model is required")
	}
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("rerank base_url is required")
	}
	provider := strings.TrimSpace(cfg.Provider)
	if provider == "" {
		provider = "openai_compatible"
	}
	timeout := defaultRerankTimeout
	if cfg.RequestTimeoutSeconds > 0 {
		timeout = time.Duration(cfg.RequestTimeoutSeconds) * time.Second
	}
	return &HTTPDocumentReranker{
		client:   &http.Client{Timeout: timeout},
		config:   cfg,
		endpoint: rerankEndpoint(baseURL),
		logger:   logger,
		provider: provider,
	}, nil
}

type HTTPDocumentReranker struct {
	client   *http.Client
	config   config.RerankConfig
	endpoint string
	logger   *zap.Logger
	provider string
}

type rerankHTTPRequest struct {
	Model           string   `json:"model"`
	Query           string   `json:"query"`
	Documents       []string `json:"documents"`
	TopN            int      `json:"top_n,omitempty"`
	ReturnDocuments bool     `json:"return_documents"`
}

type rerankHTTPResponse struct {
	Results []rerankHTTPResult `json:"results"`
}

type rerankHTTPResult struct {
	Index          int      `json:"index"`
	RelevanceScore *float64 `json:"relevance_score,omitempty"`
	Score          *float64 `json:"score,omitempty"`
}

func rerankEndpoint(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(strings.ToLower(base), "/rerank") {
		return base
	}
	return base + "/rerank"
}

func (r *HTTPDocumentReranker) Rerank(ctx context.Context, query string, docs []*schema.Document) ([]*schema.Document, error) {
	if r == nil {
		return docs, nil
	}
	q := strings.TrimSpace(query)
	if q == "" || len(docs) < 2 {
		return docs, nil
	}

	texts := make([]string, 0, len(docs))
	indexMap := make([]int, 0, len(docs))
	for i, d := range docs {
		if d == nil || strings.TrimSpace(d.Content) == "" {
			continue
		}
		texts = append(texts, d.Content)
		indexMap = append(indexMap, i)
	}
	if len(texts) < 2 {
		return docs, nil
	}

	topN := r.config.TopN
	if topN <= 0 || topN > len(texts) {
		topN = len(texts)
	}
	payload := rerankHTTPRequest{
		Model:           strings.TrimSpace(r.config.Model),
		Query:           q,
		Documents:       texts,
		TopN:            topN,
		ReturnDocuments: false,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal rerank request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new rerank request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if key := strings.TrimSpace(r.config.APIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rerank request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read rerank response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("rerank status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed rerankHTTPResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("decode rerank response: %w", err)
	}
	out := make([]*schema.Document, 0, len(parsed.Results))
	seen := make(map[int]struct{}, len(parsed.Results))
	for _, res := range parsed.Results {
		if res.Index < 0 || res.Index >= len(indexMap) {
			continue
		}
		origIdx := indexMap[res.Index]
		if _, ok := seen[origIdx]; ok {
			continue
		}
		seen[origIdx] = struct{}{}
		score := rerankResultScore(res, docs[origIdx].Score())
		out = append(out, cloneRerankedDocument(docs[origIdx], score))
	}
	for i, d := range docs {
		if _, ok := seen[i]; ok || d == nil {
			continue
		}
		out = append(out, d)
	}
	if r.logger != nil {
		r.logger.Debug("知识检索重排完成",
			zap.String("provider", r.provider),
			zap.String("model", r.config.Model),
			zap.Int("input_docs", len(docs)),
			zap.Int("ranked_docs", len(out)),
		)
	}
	return out, nil
}

func rerankResultScore(res rerankHTTPResult, fallback float64) float64 {
	if res.RelevanceScore != nil {
		return *res.RelevanceScore
	}
	if res.Score != nil {
		return *res.Score
	}
	return fallback
}

func cloneRerankedDocument(d *schema.Document, score float64) *schema.Document {
	if d == nil {
		return nil
	}
	meta := make(map[string]any, len(d.MetaData)+2)
	for k, v := range d.MetaData {
		meta[k] = v
	}
	meta["vector_score"] = d.Score()
	meta["rerank_score"] = score
	clone := &schema.Document{
		ID:       d.ID,
		Content:  d.Content,
		MetaData: meta,
	}
	clone.WithScore(score)
	return clone
}
