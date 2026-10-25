// permission_whitelist_seed_test.go D-2 的 seed 契约：
// global/agents 两档生效、缺文件静默、坏文件不半应用、未设置时行为与改动前一致。
package tooluse

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeSeedFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "whitelist-seed.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("写 seed 文件失败: %v", err)
	}
	return path
}

func TestApplyWhitelistSeedFile_GlobalAndAgentTiers(t *testing.T) {
	t.Setenv("TOOL_PERMISSION_DEFAULT_DENY", "true") // 构造出来的 checker 是 default-deny
	c := NewWhitelistPermissionChecker()
	ctx := context.Background()

	path := writeSeedFile(t, `{
		"global": ["global.tool"],
		"agents": {"agent-x": ["agent.tool", "global.tool"]}
	}`)
	res, err := ApplyWhitelistSeedFile(c, path)
	if err != nil {
		t.Fatalf("seed 加载失败: %v", err)
	}
	if !res.Loaded || res.GlobalTools != 1 || res.Agents != 1 || res.AgentTools != 2 {
		t.Fatalf("加载结果=%+v，期望 loaded/global=1/agents=1/agent_tools=2", res)
	}

	agentX := &ToolContext{AgentID: "agent-x"}
	agentY := &ToolContext{AgentID: "agent-y"}

	// 全局档：没配白名单的 Agent 命中全局白名单就放行。
	if err := c.Check(ctx, "global.tool", agentY); err != nil {
		t.Errorf("agent-y 调 global.tool 应被全局白名单放行：%v", err)
	}
	// 全局档之外仍按 default-deny 拒绝（seed 没有改默认值）。
	if err := c.Check(ctx, "agent.tool", agentY); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("agent-y 调 agent.tool 应被默认拒绝，实际 %v", err)
	}
	// Agent 档：逐条授权生效。
	if err := c.Check(ctx, "agent.tool", agentX); err != nil {
		t.Errorf("agent-x 调 agent.tool 应放行：%v", err)
	}
	if err := c.Check(ctx, "global.tool", agentX); err != nil {
		t.Errorf("agent-x 的名单里也含 global.tool，应放行：%v", err)
	}
	// Agent 白名单是权威：配了名单但没这个工具时拒绝，不回落全局/defaultAllow。
	if err := c.Check(ctx, "other.tool", agentX); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("agent-x 调名单外工具应拒绝，实际 %v", err)
	}
}

func TestApplyWhitelistSeedFile_MissingFileIsSilent(t *testing.T) {
	t.Setenv("TOOL_PERMISSION_DEFAULT_DENY", "") // 默认放行
	c := NewWhitelistPermissionChecker()
	before := c.GetDefaultAllow()

	res, err := ApplyWhitelistSeedFile(c, filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err != nil {
		t.Fatalf("文件不存在不应报错：%v", err)
	}
	if res.Loaded {
		t.Error("文件不存在却报告 Loaded=true")
	}
	if got := c.GetDefaultAllow(); got != before {
		t.Errorf("defaultAllow 被改成 %v（原 %v）", got, before)
	}
	if err := c.Check(context.Background(), "any.tool", &ToolContext{AgentID: "agent-z"}); err != nil {
		t.Errorf("未设置 seed 时默认行为应与改动前一致（放行），实际 %v", err)
	}
	if got := len(c.ListGlobalWhitelist()); got != 0 {
		t.Errorf("缺文件时全局白名单被写了 %d 条", got)
	}
}

func TestApplyWhitelistSeedFile_InvalidJSONAppliesNothing(t *testing.T) {
	t.Setenv("TOOL_PERMISSION_DEFAULT_DENY", "")
	c := NewWhitelistPermissionChecker()
	before := c.GetDefaultAllow()

	res, err := ApplyWhitelistSeedFile(c, writeSeedFile(t, `{"global": ["a.tool"],`))
	if err == nil {
		t.Fatal("坏 JSON 应返回错误")
	}
	if res.Loaded {
		t.Error("坏 JSON 却报告 Loaded=true")
	}
	if got := c.GetDefaultAllow(); got != before {
		t.Errorf("坏 JSON 改动了 defaultAllow：%v", got)
	}
	if got := len(c.ListGlobalWhitelist()); got != 0 {
		t.Errorf("坏 JSON 半应用了 %d 条全局白名单", got)
	}
	if got := len(c.ListConfiguredAgents()); got != 0 {
		t.Errorf("坏 JSON 半应用了 %d 个 Agent 白名单", got)
	}
}

func TestApplyWhitelistSeedFromEnv_UnsetKeepsDefaults(t *testing.T) {
	t.Setenv(WhitelistSeedFileEnv, "")
	t.Setenv("TOOL_PERMISSION_DEFAULT_DENY", "")
	c := NewWhitelistPermissionChecker()
	before := c.GetDefaultAllow()

	res, err := ApplyWhitelistSeedFromEnv(c)
	if err != nil {
		t.Fatalf("未设置 seed 路径不应报错：%v", err)
	}
	if res.Loaded {
		t.Error("未设置 seed 路径却报告 Loaded=true")
	}
	if got := c.GetDefaultAllow(); got != before {
		t.Errorf("defaultAllow 被改成 %v（原 %v）", got, before)
	}
	if err := c.Check(context.Background(), "any.tool", &ToolContext{AgentID: "agent-z"}); err != nil {
		t.Errorf("未设置 seed 时默认行为应与改动前一致（放行），实际 %v", err)
	}
}
