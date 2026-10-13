package app

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"hivemtk-user/internal/aiagent/agent/tooluse"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/service"
)

// wantAgentToolWired 是 agent_tool 组本轮接线的 4 条。result_cache_ttl 不在其中：
// ResultCache 的生产链路从未构造过它，没有构造点就没有可接的读取点。
var wantAgentToolWired = []string{
	"agent_tool.max_concurrent",
	"agent_tool.max_content_len",
	"agent_tool.fail_threshold",
	"agent_tool.cooldown_duration",
}

func resetAgentToolProviders(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		tooluse.SetMaxConcurrentProvider(nil)
		tooluse.SetMaxContentLenProvider(nil)
		tooluse.SetRouterFailThresholdProvider(nil)
		tooluse.SetRouterCooldownDurationProvider(nil)
	})
}

func newAgentToolParamFixture(t *testing.T) *service.ConfigParamService {
	t.Helper()
	resetAgentToolProviders(t)
	database := testutil.NewTestDBOrSkip(t, &model.ConfigParam{}, &model.ConfigParamAuditLog{})
	if database == nil {
		t.Skip("no DB")
	}
	if err := service.SeedConfigParams(context.Background(), database); err != nil {
		t.Fatalf("SeedConfigParams 失败：%v", err)
	}
	svc := service.NewConfigParamService(database)
	service.SetGlobalForTest(svc)
	t.Cleanup(func() { service.SetGlobalForTest(nil) })
	return svc
}

func TestWireAgentToolConfigParamsKeys(t *testing.T) {
	resetAgentToolProviders(t)
	got := WireAgentToolConfigParams()
	if len(got) != len(wantAgentToolWired) {
		t.Fatalf("接线条数 = %d，期望 %d\ngot  = %v\nwant = %v", len(got), len(wantAgentToolWired), got, wantAgentToolWired)
	}
	seen := map[string]bool{}
	for i, k := range got {
		if k != wantAgentToolWired[i] {
			t.Errorf("第 %d 条接线 = %q，期望 %q", i, k, wantAgentToolWired[i])
		}
		if seen[k] {
			t.Errorf("键 %q 被接了两次", k)
		}
		seen[k] = true
	}
}

func TestWiredAgentToolKeysExistInSeed(t *testing.T) {
	index := map[string]bool{}
	for _, d := range service.DefaultParamDefs() {
		index[d.Group+"."+d.Key] = true
	}
	for _, k := range wantAgentToolWired {
		if !index[k] {
			t.Errorf("%q 在 DefaultParamDefs() 里不存在", k)
		}
	}
	// 反向钉死：result_cache_ttl 必须仍在种子里，且必须仍带「未接线」标注。
	// 它没有生产构造点，接线只是给别人一个「改了会生效」的错觉。
	var found bool
	for _, d := range service.DefaultParamDefs() {
		if d.Group == "agent_tool" && d.Key == "result_cache_ttl" {
			found = true
		}
	}
	if !found {
		t.Error("agent_tool.result_cache_ttl 从种子里消失了：它没有生产构造点，应保留条目与「未接线」标注")
	}
}

func TestWireAgentToolConfigParamsReachesReaders(t *testing.T) {
	svc := newAgentToolParamFixture(t)
	ctx := context.Background()

	beforeConcurrent := tooluse.MaxConcurrent()
	beforeContentLen := tooluse.MaxContentLen()
	beforeThreshold := tooluse.RouterFailThreshold()
	beforeCooldown := tooluse.RouterCooldownDuration()

	WireAgentToolConfigParams()

	for _, tc := range []struct{ key, val string }{
		{"max_concurrent", "12"},
		{"max_content_len", "8000"},
		{"fail_threshold", "8"},
		{"cooldown_duration", "120"},
	} {
		if err := svc.UpdateValue(ctx, "agent_tool", tc.key, tc.val, 1); err != nil {
			t.Fatalf("改 agent_tool.%s 失败：%v", tc.key, err)
		}
	}

	if got, want := tooluse.MaxConcurrent(), 12; got != want {
		t.Errorf("max_concurrent 未被读走：got %v（原 %v）want %v", got, beforeConcurrent, want)
	}
	if got, want := tooluse.MaxContentLen(), 8000; got != want {
		t.Errorf("max_content_len 未被读走：got %v（原 %v）want %v", got, beforeContentLen, want)
	}
	if got, want := tooluse.RouterFailThreshold(), 8; got != want {
		t.Errorf("fail_threshold 未被读走：got %v（原 %v）want %v", got, beforeThreshold, want)
	}
	if got, want := tooluse.RouterCooldownDuration(), 120*time.Second; got != want {
		t.Errorf("cooldown_duration 未被读走：got %v（原 %v）want %v", got, beforeCooldown, want)
	}
}

func TestWireAgentToolConfigParamsWithoutDB(t *testing.T) {
	service.SetGlobalForTest(nil)
	resetAgentToolProviders(t)

	if got := WireAgentToolConfigParams(); len(got) != len(wantAgentToolWired) {
		t.Fatalf("无库时接线条数 = %d，期望 %d", len(got), len(wantAgentToolWired))
	}
	if got, want := tooluse.MaxConcurrent(), tooluse.DefaultMaxConcurrent; got != want {
		t.Errorf("无库时 max_concurrent = %v，期望兜底 %v", got, want)
	}
	if got, want := tooluse.MaxContentLen(), tooluse.DefaultMaxContentLen; got != want {
		t.Errorf("无库时 max_content_len = %v，期望兜底 %v", got, want)
	}
	if got, want := tooluse.RouterFailThreshold(), tooluse.DefaultRouterFailThreshold; got != want {
		t.Errorf("无库时 fail_threshold = %v，期望兜底 %v", got, want)
	}
	if got, want := tooluse.RouterCooldownDuration(), tooluse.DefaultRouterCooldownDuration; got != want {
		t.Errorf("无库时 cooldown_duration = %v，期望兜底 %v", got, want)
	}
}

// TestToolRouterConstructionReadsAccessors 装配点必须把 accessor 的返回值传进 RouterConfig。
//
// 这是一条静态锁，因为「构造时到底取了谁」在行为层面读不出来：RouterConfig 的
// FailThreshold/CooldownDuration 落进 ToolRouter 的私有字段，GetStats() 不回传它们，
// 而 accessor 自身的取值行为已由 tooluse 的 seam 用例与本包上面两条用例钉住。
//
// 它守的是原病灶：provider 注进去、参数中心页面上也能改，但 `NewToolRouter` 的调用点
// 传的是字面量 5 / 30 * time.Second，而 NewToolRouter 只在**非正值**时才回落 accessor
// （tool_router.go:58,61）⇒ 正数字面量把那条回落路堵死，注入永远不参与决策。
//
// 判据按行且跳过 `//` 开头的注释行：装配点上方那段解释性注释随时可能写成
// 「这里必须调 tooluse.RouterFailThreshold()」，整段子串计数就会从 1 变 2 而报假红。
// 反向验过：往文件里加这样一条注释，本用例仍绿（注释不进计数）；
// 把 accessor 调用换成字面量，本用例立刻红（A 腿 0 次）。
func TestToolRouterConstructionReadsAccessors(t *testing.T) {
	src, err := os.ReadFile("tool_executor_wiring.go")
	if err != nil {
		t.Fatalf("读 tool_executor_wiring.go 失败: %v", err)
	}
	elems := strings.Split(string(src), "\n")
	code := make([]string, 0, len(elems))
	lineNos := make([]int, 0, len(elems))
	for i, line := range elems {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		code = append(code, line)
		lineNos = append(lineNos, i+1)
	}
	for _, accessor := range []string{"tooluse.RouterFailThreshold()", "tooluse.RouterCooldownDuration()"} {
		n := 0
		for _, line := range code {
			if strings.Contains(line, accessor) {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("%s 在装配点出现 %d 次（期望恰好 1 次）⇒ 要么 RouterConfig 没取参数中心生效值，要么有两处构造点互相覆盖", accessor, n)
		}
	}
	// RouterConfig 的两个字段只能接局部变量，接字面量即判红。
	for _, tc := range []struct{ field, wantVar string }{
		{"FailThreshold:", "failThreshold"},
		{"CooldownDuration:", "cooldownDuration"},
	} {
		for i, line := range code {
			if !strings.Contains(line, tc.field) {
				continue
			}
			if !strings.Contains(line, tc.wantVar) {
				t.Errorf("第 %d 行把 %s 接到了字面量（该行应只接 %s）：%s",
					lineNos[i], tc.field, tc.wantVar, strings.TrimSpace(line))
			}
		}
	}
}
