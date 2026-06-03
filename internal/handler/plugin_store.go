package handler

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"cyberstrike-ai/internal/config"
	"cyberstrike-ai/internal/mcp"
	"cyberstrike-ai/internal/pluginstore"
	"cyberstrike-ai/internal/security"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type PluginStoreHandler struct {
	manager    *pluginstore.Manager
	config     *config.Config
	configPath string
	mcpServer  *mcp.Server
	executor   *security.Executor
	logger     *zap.Logger
	mu         sync.Mutex
}

type PluginSourceRequest struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type PluginInstallRequest struct {
	Source   string `json:"source"`
	PluginID string `json:"plugin_id"`
}

type PluginStoreSettingsRequest struct {
	GitHubToken      string `json:"github_token"`
	ClearGitHubToken bool   `json:"clear_github_token"`
}

type PluginStoreSettingsResponse struct {
	Enabled               bool   `json:"enabled"`
	RootDir               string `json:"root_dir"`
	GitHubTokenConfigured bool   `json:"github_token_configured"`
}

func NewPluginStoreHandler(manager *pluginstore.Manager, cfg *config.Config, configPath string, mcpServer *mcp.Server, executor *security.Executor, logger *zap.Logger) *PluginStoreHandler {
	return &PluginStoreHandler{
		manager:    manager,
		config:     cfg,
		configPath: configPath,
		mcpServer:  mcpServer,
		executor:   executor,
		logger:     logger,
	}
}

func (h *PluginStoreHandler) GetSettings(c *gin.Context) {
	if !h.enabled(c) {
		return
	}
	cfg := config.PluginStoreConfig{}
	if h.config != nil {
		cfg = h.config.PluginStore
	}
	c.JSON(http.StatusOK, pluginStoreSettingsResponse(cfg))
}

func (h *PluginStoreHandler) UpdateSettings(c *gin.Context) {
	if !h.enabled(c) {
		return
	}
	var req PluginStoreSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误: " + err.Error()})
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.config == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "配置未初始化"})
		return
	}

	cfg := h.config.PluginStore
	if req.ClearGitHubToken {
		cfg.GitHubToken = ""
	} else if token := strings.TrimSpace(req.GitHubToken); token != "" {
		cfg.GitHubToken = token
	}
	h.config.PluginStore = cfg
	if h.manager != nil {
		h.manager.SetGitHubToken(cfg.GitHubToken)
	}
	if err := h.saveConfig(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存插件商店配置失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, pluginStoreSettingsResponse(cfg))
}

func (h *PluginStoreHandler) GetSources(c *gin.Context) {
	if !h.enabled(c) {
		return
	}
	sources, err := h.manager.ListSources()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sources": sources})
}

func (h *PluginStoreHandler) AddSource(c *gin.Context) {
	if !h.enabled(c) {
		return
	}
	var req PluginSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误: " + err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Minute)
	defer cancel()
	source, err := h.manager.AddOrSyncSource(ctx, strings.TrimSpace(req.Name), strings.TrimSpace(req.URL))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"source": source})
}

func (h *PluginStoreHandler) GetCatalog(c *gin.Context) {
	if !h.enabled(c) {
		return
	}
	source := strings.TrimSpace(c.Query("source"))
	if source != "" {
		catalog, err := h.manager.CatalogForSource(source)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"catalogs": []pluginstore.Catalog{*catalog}})
		return
	}
	catalogs, err := h.manager.Catalogs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"catalogs": catalogs})
}

func (h *PluginStoreHandler) GetInstalled(c *gin.Context) {
	if !h.enabled(c) {
		return
	}
	installed, err := h.manager.ListInstalled()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"installed": installed})
}

func (h *PluginStoreHandler) InstallPlugin(c *gin.Context) {
	if !h.enabled(c) {
		return
	}
	var req PluginInstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误: " + err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
	defer cancel()
	installed, err := h.manager.InstallPlugin(ctx, strings.TrimSpace(req.Source), strings.TrimSpace(req.PluginID))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	loaded, skipped, err := h.reloadInstalledTools()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "插件已安装，但重新注册工具失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"installed": installed,
		"loaded":    loaded,
		"skipped":   skipped,
	})
}

func (h *PluginStoreHandler) Reload(c *gin.Context) {
	if !h.enabled(c) {
		return
	}
	loaded, skipped, err := h.reloadInstalledTools()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"loaded": loaded, "skipped": skipped})
}

func (h *PluginStoreHandler) reloadInstalledTools() ([]string, []string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	pluginTools, err := h.manager.LoadInstalledTools()
	if err != nil {
		return nil, nil, err
	}
	if h.config != nil {
		h.config.Security.ExtraPathDirs = mergePluginPathDirs(h.config.Security.ExtraPathDirs, h.manager.RuntimeBinDirs())
		merged, skipped := pluginstore.MergeTools(h.config.Security.Tools, pluginTools)
		h.config.Security.Tools = merged
		if h.executor != nil && h.mcpServer != nil {
			h.executor.RegisterTools(h.mcpServer)
		}
		loaded := make([]string, 0, len(pluginTools))
		for _, tool := range pluginTools {
			loaded = append(loaded, tool.Name)
		}
		if h.logger != nil {
			h.logger.Info("插件工具已重新注册",
				zap.Int("loaded", len(loaded)),
				zap.Int("skipped", len(skipped)),
			)
		}
		return loaded, skipped, nil
	}
	return nil, nil, nil
}

func (h *PluginStoreHandler) enabled(c *gin.Context) bool {
	if h == nil || h.manager == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "插件商店未启用"})
		return false
	}
	return true
}

func pluginStoreSettingsResponse(cfg config.PluginStoreConfig) PluginStoreSettingsResponse {
	return PluginStoreSettingsResponse{
		Enabled:               cfg.EnabledEffective(),
		RootDir:               cfg.RootDirEffective(),
		GitHubTokenConfigured: strings.TrimSpace(cfg.GitHubToken) != "",
	}
}

func (h *PluginStoreHandler) saveConfig() error {
	if h.configPath == "" || h.config == nil {
		return nil
	}
	doc, err := loadYAMLDocument(h.configPath)
	if err != nil {
		return err
	}
	if err := writeRedactedPluginStoreBackup(h.configPath, doc); err != nil {
		return fmt.Errorf("创建配置备份失败: %w", err)
	}
	updatePluginStoreConfig(doc, h.config.PluginStore)
	if err := writeYAMLDocument(h.configPath, doc); err != nil {
		return err
	}
	if h.logger != nil {
		h.logger.Info("插件商店配置已保存", zap.String("path", h.configPath))
	}
	return nil
}

func writeRedactedPluginStoreBackup(path string, doc *yaml.Node) error {
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	backup := cloneYAMLNode(doc)
	if backup != nil && backup.Kind == yaml.DocumentNode && len(backup.Content) > 0 {
		storeNode := findMapValue(backup.Content[0], "plugin_store")
		if storeNode != nil {
			setStringInMap(storeNode, "github_token", "")
		}
	}
	var buf strings.Builder
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(backup); err != nil {
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	return os.WriteFile(path+".backup", []byte(buf.String()), 0o600)
}

func cloneYAMLNode(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	cloned := *node
	if len(node.Content) > 0 {
		cloned.Content = make([]*yaml.Node, len(node.Content))
		for i, child := range node.Content {
			cloned.Content[i] = cloneYAMLNode(child)
		}
	}
	return &cloned
}

func updatePluginStoreConfig(doc *yaml.Node, cfg config.PluginStoreConfig) {
	root := doc.Content[0]
	storeNode := ensureMap(root, "plugin_store")
	setBoolInMap(storeNode, "enabled", cfg.EnabledEffective())
	setStringInMap(storeNode, "root_dir", cfg.RootDirEffective())
	setStringInMap(storeNode, "github_token", strings.TrimSpace(cfg.GitHubToken))
}

func mergePluginPathDirs(base []string, extra []string) []string {
	out := append([]string(nil), base...)
	seen := make(map[string]struct{}, len(base)+len(extra))
	for _, dir := range out {
		key := strings.TrimSpace(dir)
		if key != "" {
			seen[key] = struct{}{}
		}
	}
	for _, dir := range extra {
		key := strings.TrimSpace(dir)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}
