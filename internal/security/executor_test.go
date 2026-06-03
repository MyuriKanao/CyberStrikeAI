package security

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cyberstrike-ai/internal/config"
	"cyberstrike-ai/internal/mcp"
	"cyberstrike-ai/internal/storage"

	"go.uber.org/zap"
)

// setupTestExecutor 创建测试用的执行器
func setupTestExecutor(t *testing.T) (*Executor, *mcp.Server) {
	logger := zap.NewNop()
	mcpServer := mcp.NewServer(logger)

	cfg := &config.SecurityConfig{
		Tools: []config.ToolConfig{},
	}

	executor := NewExecutor(cfg, mcpServer, logger)
	return executor, mcpServer
}

// setupTestStorage 创建测试用的存储
func setupTestStorage(t *testing.T) *storage.FileResultStorage {
	tmpDir := filepath.Join(os.TempDir(), "test_executor_storage_"+time.Now().Format("20060102_150405"))
	logger := zap.NewNop()

	storage, err := storage.NewFileResultStorage(tmpDir, logger)
	if err != nil {
		t.Fatalf("创建测试存储失败: %v", err)
	}

	return storage
}

func TestExecutor_ExecuteInternalTool_QueryExecutionResult(t *testing.T) {
	executor, _ := setupTestExecutor(t)
	testStorage := setupTestStorage(t)
	executor.SetResultStorage(testStorage)

	// 准备测试数据
	executionID := "test_exec_001"
	toolName := "nmap_scan"
	result := "Line 1: Port 22 open\nLine 2: Port 80 open\nLine 3: Port 443 open\nLine 4: error occurred"

	// 保存测试结果
	err := testStorage.SaveResult(executionID, toolName, result)
	if err != nil {
		t.Fatalf("保存测试结果失败: %v", err)
	}

	ctx := context.Background()

	// 测试1: 基本查询（第一页）
	args := map[string]interface{}{
		"execution_id": executionID,
		"page":         float64(1),
		"limit":        float64(2),
	}

	toolResult, err := executor.executeQueryExecutionResult(ctx, args)
	if err != nil {
		t.Fatalf("执行查询失败: %v", err)
	}

	if toolResult.IsError {
		t.Fatalf("查询应该成功，但返回了错误: %s", toolResult.Content[0].Text)
	}

	// 验证结果包含预期内容
	resultText := toolResult.Content[0].Text
	if !strings.Contains(resultText, executionID) {
		t.Errorf("结果中应该包含执行ID: %s", executionID)
	}

	if !strings.Contains(resultText, "第 1/") {
		t.Errorf("结果中应该包含分页信息")
	}

	// 测试2: 搜索功能
	args2 := map[string]interface{}{
		"execution_id": executionID,
		"search":       "error",
		"page":         float64(1),
		"limit":        float64(10),
	}

	toolResult2, err := executor.executeQueryExecutionResult(ctx, args2)
	if err != nil {
		t.Fatalf("执行搜索失败: %v", err)
	}

	if toolResult2.IsError {
		t.Fatalf("搜索应该成功，但返回了错误: %s", toolResult2.Content[0].Text)
	}

	resultText2 := toolResult2.Content[0].Text
	if !strings.Contains(resultText2, "error") {
		t.Errorf("搜索结果中应该包含关键词: error")
	}

	// 测试3: 过滤功能
	args3 := map[string]interface{}{
		"execution_id": executionID,
		"filter":       "Port",
		"page":         float64(1),
		"limit":        float64(10),
	}

	toolResult3, err := executor.executeQueryExecutionResult(ctx, args3)
	if err != nil {
		t.Fatalf("执行过滤失败: %v", err)
	}

	if toolResult3.IsError {
		t.Fatalf("过滤应该成功，但返回了错误: %s", toolResult3.Content[0].Text)
	}

	resultText3 := toolResult3.Content[0].Text
	if !strings.Contains(resultText3, "Port") {
		t.Errorf("过滤结果中应该包含关键词: Port")
	}

	// 测试4: 缺少必需参数
	args4 := map[string]interface{}{
		"page": float64(1),
	}

	toolResult4, err := executor.executeQueryExecutionResult(ctx, args4)
	if err != nil {
		t.Fatalf("执行查询失败: %v", err)
	}

	if !toolResult4.IsError {
		t.Fatal("缺少execution_id应该返回错误")
	}

	// 测试5: 不存在的执行ID
	args5 := map[string]interface{}{
		"execution_id": "nonexistent_id",
		"page":         float64(1),
	}

	toolResult5, err := executor.executeQueryExecutionResult(ctx, args5)
	if err != nil {
		t.Fatalf("执行查询失败: %v", err)
	}

	if !toolResult5.IsError {
		t.Fatal("不存在的执行ID应该返回错误")
	}
}

func TestExecutor_ExecuteInternalTool_UnknownTool(t *testing.T) {
	executor, _ := setupTestExecutor(t)

	ctx := context.Background()
	args := map[string]interface{}{
		"test": "value",
	}

	// 测试未知的内部工具类型
	toolResult, err := executor.executeInternalTool(ctx, "unknown_tool", "internal:unknown_tool", args)
	if err != nil {
		t.Fatalf("执行内部工具失败: %v", err)
	}

	if !toolResult.IsError {
		t.Fatal("未知的工具类型应该返回错误")
	}

	if !strings.Contains(toolResult.Content[0].Text, "未知的内部工具类型") {
		t.Errorf("错误消息应该包含'未知的内部工具类型'")
	}
}

func TestExecutor_ExecuteInternalTool_NoStorage(t *testing.T) {
	executor, _ := setupTestExecutor(t)
	// 不设置存储，测试未初始化的情况

	ctx := context.Background()
	args := map[string]interface{}{
		"execution_id": "test_id",
	}

	toolResult, err := executor.executeQueryExecutionResult(ctx, args)
	if err != nil {
		t.Fatalf("执行查询失败: %v", err)
	}

	if !toolResult.IsError {
		t.Fatal("未初始化的存储应该返回错误")
	}

	if !strings.Contains(toolResult.Content[0].Text, "结果存储未初始化") {
		t.Errorf("错误消息应该包含'结果存储未初始化'")
	}
}

func TestBuildCommandArgs_UsesParameterAliases(t *testing.T) {
	executor, _ := setupTestExecutor(t)
	tool := &config.ToolConfig{
		Name:    "dirsearch",
		Command: "dirsearch",
		Parameters: []config.ParameterConfig{
			{
				Name:     "target",
				Aliases:  []string{"url"},
				Type:     "string",
				Required: true,
				Flag:     "-u",
				Format:   "flag",
			},
		},
	}

	args := executor.buildCommandArgs("dirsearch", tool, map[string]interface{}{
		"url": "http://127.0.0.1:8080/",
	})

	want := []string{"-u", "http://127.0.0.1:8080/"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %q, want %q", args, want)
	}
}

func TestNmapAcceptsCommonLLMArgumentAliases(t *testing.T) {
	executor, _ := setupTestExecutor(t)
	tool, err := config.LoadToolFromFile(filepath.Join("..", "..", "tools", "nmap.yaml"))
	if err != nil {
		t.Fatalf("LoadToolFromFile(nmap): %v", err)
	}

	args := executor.buildCommandArgs("nmap", tool, map[string]interface{}{
		"ports":   "12527",
		"scan":    "-sV",
		"targets": "192.168.50.147",
	})

	want := []string{"-p", "12527", "-sV", "192.168.50.147"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %q, want %q", args, want)
	}
}

func TestBuildCommandArgs_PositionalActionIsPassedToCommand(t *testing.T) {
	executor, _ := setupTestExecutor(t)
	pos0 := 0
	pos2 := 2
	tool := &config.ToolConfig{
		Name:    "dnslog",
		Command: "python3",
		Args:    []string{"-c", "script"},
		Parameters: []config.ParameterConfig{
			{
				Name:     "action",
				Type:     "string",
				Required: false,
				Default:  "get_domain",
				Position: &pos0,
				Format:   "positional",
			},
			{
				Name:     "wait_time",
				Type:     "int",
				Required: false,
				Default:  0,
				Position: &pos2,
				Format:   "positional",
			},
		},
	}

	args := executor.buildCommandArgs("dnslog", tool, map[string]interface{}{
		"action": "get_domain",
	})

	wantPrefix := []string{"-c", "script", "get_domain"}
	if len(args) < len(wantPrefix) {
		t.Fatalf("args = %q, want prefix %q", args, wantPrefix)
	}
	if strings.Join(args[:len(wantPrefix)], "\x00") != strings.Join(wantPrefix, "\x00") {
		t.Fatalf("args = %q, want prefix %q", args, wantPrefix)
	}
}

func TestHydraHTTPFormToolAcceptsLLMStyleArguments(t *testing.T) {
	executor, _ := setupTestExecutor(t)
	tool, err := config.LoadToolFromFile(filepath.Join("..", "..", "tools", "hydra.yaml"))
	if err != nil {
		t.Fatalf("LoadToolFromFile(hydra): %v", err)
	}

	if tool.Command != "python3" {
		t.Fatalf("hydra should use the Python compatibility wrapper, got command %q", tool.Command)
	}

	schema := executor.buildInputSchema(tool)
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("hydra schema properties missing: %#v", schema)
	}
	for _, name := range []string{"url", "target", "service", "credential_lists", "password_list", "success_identifier", "failure_identifier"} {
		if _, ok := props[name]; !ok {
			t.Fatalf("hydra schema missing LLM-friendly property %q: %#v", name, props)
		}
	}

	args := executor.buildCommandArgs("hydra", tool, map[string]interface{}{
		"target":             "http-post-form",
		"url":                "http://192.168.50.147:8080/api/v1/auth/login",
		"method":             "POST",
		"credential_lists":   []interface{}{"admin@sub2api.local"},
		"password_list":      []interface{}{"admin", "admin123"},
		"failure_identifier": "INVALID_CREDENTIALS",
		"success_identifier": "token",
		"threads":            float64(4),
		"show_command":       true,
	})
	joined := strings.Join(args, "\x00")
	for _, want := range []string{
		"--url\x00http://192.168.50.147:8080/api/v1/auth/login",
		"--target\x00http-post-form",
		"--credential-lists\x00admin@sub2api.local",
		"--password-list\x00admin,admin123",
		"--failure-identifier\x00INVALID_CREDENTIALS",
		"--success-identifier\x00token",
		"--threads\x004",
		"--show-command",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("hydra args %q missing %q", args, want)
		}
	}
}

func TestHydraHTTPFormWrapperFindsPasswordWithoutHydraBinary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method == http.MethodPost &&
			strings.Contains(string(body), "admin@sub2api.local") &&
			strings.Contains(string(body), "secret") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"ok"}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`INVALID_CREDENTIALS`))
	}))
	defer srv.Close()

	tool, err := config.LoadToolFromFile(filepath.Join("..", "..", "tools", "hydra.yaml"))
	if err != nil {
		t.Fatalf("LoadToolFromFile(hydra): %v", err)
	}
	cfg := &config.SecurityConfig{Tools: []config.ToolConfig{*tool}}
	executor := NewExecutor(cfg, mcp.NewServer(zap.NewNop()), zap.NewNop())

	res, err := executor.ExecuteTool(context.Background(), "hydra", map[string]interface{}{
		"target":             "http-post-form",
		"url":                srv.URL + "/api/v1/auth/login",
		"method":             "POST",
		"credential_lists":   []interface{}{"admin@sub2api.local"},
		"password_list":      []interface{}{"bad", "secret"},
		"failure_identifier": "INVALID_CREDENTIALS",
		"success_identifier": "token",
		"threads":            float64(2),
	})
	if err != nil {
		t.Fatalf("ExecuteTool(hydra): %v", err)
	}
	if res == nil || res.IsError {
		t.Fatalf("hydra HTTP wrapper should succeed, got %+v", res)
	}
	text := res.Content[0].Text
	for _, want := range []string{"success", "admin@sub2api.local", "secret"} {
		if !strings.Contains(text, want) {
			t.Fatalf("hydra output %q missing %q", text, want)
		}
	}
}

func TestBuildCommandArgs_RequiredPositionZeroMissingReturnsEmpty(t *testing.T) {
	executor, _ := setupTestExecutor(t)
	pos0 := 0
	pos1 := 1
	tool := &config.ToolConfig{
		Name:    "execute-python-script",
		Command: "/bin/bash",
		Args:    []string{"-c", "runner", "_"},
		Parameters: []config.ParameterConfig{
			{
				Name:     "code",
				Type:     "string",
				Required: true,
				Position: &pos0,
				Format:   "positional",
			},
			{
				Name:     "env_name",
				Type:     "string",
				Required: false,
				Default:  "default",
				Position: &pos1,
				Format:   "positional",
			},
		},
	}

	args := executor.buildCommandArgs("execute-python-script", tool, map[string]interface{}{})

	if len(args) != 0 {
		t.Fatalf("args = %q, want empty args for missing required position 0", args)
	}
}

func TestExecuteSystemCommand_BackgroundDoesNotBlockOnChildStdout(t *testing.T) {
	executor, _ := setupTestExecutor(t)
	// 子进程先向 stdout 写无换行字符再长时间 sleep；若与 echo $pid 共享管道且未重定向子进程 stdout，
	// ReadString('\n') 会阻塞到子进程退出。后台包装须将子进程标准流与 PID 行分离。
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	args := map[string]interface{}{
		"command": `(sh -c 'printf x; sleep 120') &`,
		"shell":   "sh",
	}
	res, err := executor.executeSystemCommand(ctx, args)
	if err != nil {
		t.Fatalf("executeSystemCommand: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatalf("expected success, got %+v", res)
	}
	txt := res.Content[0].Text
	if !strings.Contains(txt, "后台命令已启动") {
		t.Fatalf("unexpected body: %q", txt)
	}
}

func TestShellBackgroundOperatorDetection(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		contains bool
		full     bool
	}{
		{
			name:     "full background",
			command:  "python3 -m http.server 8888 &",
			contains: true,
			full:     true,
		},
		{
			name:     "mixed background and foreground without whitespace after ampersand",
			command:  `python3 -m http.server 8888 &(sleep 2); echo ready`,
			contains: true,
			full:     false,
		},
		{
			name:     "logical and is not background",
			command:  "echo one && echo two",
			contains: false,
			full:     false,
		},
		{
			name:     "stderr redirect is not background",
			command:  "python3 app.py 2>&1",
			contains: false,
			full:     false,
		},
		{
			name:     "bash combined redirect is not background",
			command:  "python3 app.py &> server.log",
			contains: false,
			full:     false,
		},
		{
			name:     "quoted ampersand is not background",
			command:  `printf '%s\n' 'a&b'`,
			contains: false,
			full:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ContainsShellBackgroundOperator(tt.command); got != tt.contains {
				t.Fatalf("ContainsShellBackgroundOperator(%q) = %v, want %v", tt.command, got, tt.contains)
			}
			if got := IsBackgroundShellCommand(tt.command); got != tt.full {
				t.Fatalf("IsBackgroundShellCommand(%q) = %v, want %v", tt.command, got, tt.full)
			}
		})
	}
}

func TestPaginateLines(t *testing.T) {
	lines := []string{"Line 1", "Line 2", "Line 3", "Line 4", "Line 5"}

	// 测试第一页
	page := paginateLines(lines, 1, 2)
	if page.Page != 1 {
		t.Errorf("页码不匹配。期望: 1, 实际: %d", page.Page)
	}
	if page.Limit != 2 {
		t.Errorf("每页行数不匹配。期望: 2, 实际: %d", page.Limit)
	}
	if page.TotalLines != 5 {
		t.Errorf("总行数不匹配。期望: 5, 实际: %d", page.TotalLines)
	}
	if page.TotalPages != 3 {
		t.Errorf("总页数不匹配。期望: 3, 实际: %d", page.TotalPages)
	}
	if len(page.Lines) != 2 {
		t.Errorf("第一页行数不匹配。期望: 2, 实际: %d", len(page.Lines))
	}

	// 测试第二页
	page2 := paginateLines(lines, 2, 2)
	if len(page2.Lines) != 2 {
		t.Errorf("第二页行数不匹配。期望: 2, 实际: %d", len(page2.Lines))
	}
	if page2.Lines[0] != "Line 3" {
		t.Errorf("第二页第一行不匹配。期望: Line 3, 实际: %s", page2.Lines[0])
	}

	// 测试最后一页
	page3 := paginateLines(lines, 3, 2)
	if len(page3.Lines) != 1 {
		t.Errorf("第三页行数不匹配。期望: 1, 实际: %d", len(page3.Lines))
	}

	// 测试超出范围的页码（应该返回最后一页）
	page4 := paginateLines(lines, 4, 2)
	if page4.Page != 3 {
		t.Errorf("超出范围的页码应该被修正为最后一页。期望: 3, 实际: %d", page4.Page)
	}
	if len(page4.Lines) != 1 {
		t.Errorf("最后一页应该只有1行。实际: %d行", len(page4.Lines))
	}

	// 测试无效页码（小于1）
	page0 := paginateLines(lines, 0, 2)
	if page0.Page != 1 {
		t.Errorf("无效页码应该被修正为1。实际: %d", page0.Page)
	}

	// 测试空列表
	emptyPage := paginateLines([]string{}, 1, 10)
	if emptyPage.TotalLines != 0 {
		t.Errorf("空列表的总行数应该为0。实际: %d", emptyPage.TotalLines)
	}
	if len(emptyPage.Lines) != 0 {
		t.Errorf("空列表应该返回空结果。实际: %d行", len(emptyPage.Lines))
	}
}
