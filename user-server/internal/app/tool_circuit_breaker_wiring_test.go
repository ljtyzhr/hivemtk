// tool_circuit_breaker_wiring_test.go T-P1-04：工具熔断接线的三条契约。
//
// 卡面 AC：
//
//	①注入连续失败后审计日志出现熔断判定记录 → TestToolCircuitDecisionLandsInAuditLog
//	  （真抓日志：把 os.Stdout 换成管道，再跑够触发判定的调用）
//	②shadow 模式下放行行为不变（有断言测试） → TestToolCircuitShadowKeepsToolCallsIdentical
//	③熔断阈值走配置不硬编码 → TestParseToolCircuitMode / TestToolCircuitConfigFromEnv
//
// 另有一条不属于卡面但更要紧的：关旗时 executor 配置必须回到"没人接熔断"的原样
// （TestApplyToolCircuitBreakerOffLeavesExecutorUnchanged），否则"默认 off"只是注释里的承诺。
package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/aiagent/agent/tooluse"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/service"
)

// restoreCircuitState 保存/还原包级熔断状态。
//
// 必须还原：本包其它测试（以及未来的接线测试）会读同一组全局变量，
// 只靠 t.Setenv 复原环境变量不够——applyToolCircuitBreaker 已经把 registry 写进去了。
func restoreCircuitState(t *testing.T) {
	t.Helper()
	om, oc, or, od := circuitModeValue, circuitConfigApplied, circuitRegistry, circuitDecisions
	t.Cleanup(func() {
		circuitModeValue, circuitConfigApplied, circuitRegistry, circuitDecisions = om, oc, or, od
	})
}

type circuitProbeTool struct {
	tooluse.BaseTool
	mu    sync.Mutex
	calls int
}

func newCircuitProbeTool(name string) *circuitProbeTool {
	return &circuitProbeTool{BaseTool: tooluse.BaseTool{
		NameVal:     name,
		CategoryVal: tooluse.CategoryCustomer,
		ParamsVal:   tooluse.ToolParameters{Type: "object", Properties: map[string]tooluse.ToolParam{}},
	}}
}

func (p *circuitProbeTool) Execute(ctx context.Context, args map[string]any) (tooluse.ToolResult, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	err := errors.New("downstream unavailable")
	return tooluse.ErrorResult(p.NameVal, err), err
}

func (p *circuitProbeTool) called() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func newExecutorWithCircuit(t *testing.T, cfg tooluse.ToolExecutorConfig) (*tooluse.ToolExecutor, *circuitProbeTool, string) {
	t.Helper()
	name := "circuit.probe"
	tool := newCircuitProbeTool(name)
	reg := tooluse.NewToolRegistry()
	if err := reg.Register(tool); err != nil {
		t.Fatalf("注册探针工具失败：%v", err)
	}
	cfg.DefaultTimeout = 5 * time.Second
	return tooluse.NewToolExecutor(reg, cfg), tool, name
}

func TestParseToolCircuitMode(t *testing.T) {
	cases := []struct {
		raw  string
		want toolCircuitMode
	}{
		{"", toolCircuitOff},
		{"   ", toolCircuitOff},
		{"off", toolCircuitOff},
		{"OFF", toolCircuitOff},
		{"false", toolCircuitOff},
		{"0", toolCircuitOff},
		{"no", toolCircuitOff},
		{"disabled", toolCircuitOff},
		{"shadow", toolCircuitShadow},
		{" Shadow ", toolCircuitShadow},
		{"observe", toolCircuitShadow},
		{"watch", toolCircuitShadow},
		{"enforce", toolCircuitEnforce},
		{"BLOCK", toolCircuitEnforce},
		{"active", toolCircuitEnforce},
		// 布尔真值只给 shadow：一把能改生产行为的旗子不该因有人按习惯写 true 就拿到拦截能力
		{"true", toolCircuitShadow},
		{"TRUE", toolCircuitShadow},
		{"1", toolCircuitShadow},
		{"yes", toolCircuitShadow},
		{"on", toolCircuitShadow},
		{"y", toolCircuitShadow},
		// 认不出一律判关
		{" enable ", toolCircuitOff},
		{"enable", toolCircuitOff},
		{"ture", toolCircuitOff},
		{"2", toolCircuitOff},
		{"-1", toolCircuitOff},
		{"yesno", toolCircuitOff},
		{"影子", toolCircuitOff},
	}
	for _, tt := range cases {
		if got := parseToolCircuitMode(tt.raw); got != tt.want {
			t.Errorf("parseToolCircuitMode(%q) = %s，期望 %s", tt.raw, got, tt.want)
		}
	}
}

func TestToolCircuitConfigFromEnvDefaults(t *testing.T) {
	for _, k := range []string{
		circuitThresholdEnv, circuitBaseCooldownEnv, circuitMaxCooldownEnv, circuitBackoffEnv, circuitHalfOpenEnv,
	} {
		t.Setenv(k, "")
	}
	if got := toolCircuitConfigFromEnv(); got != tooluse.DefaultCircuitBreakerConfig() {
		t.Fatalf("未配环境变量时应逐字沿用 tooluse 默认配置，实际 %+v", got)
	}
}

func TestToolCircuitConfigFromEnvOverrides(t *testing.T) {
	t.Setenv(circuitThresholdEnv, "12")
	t.Setenv(circuitBaseCooldownEnv, "45s")
	t.Setenv(circuitMaxCooldownEnv, "12m")
	t.Setenv(circuitBackoffEnv, "3.5")
	t.Setenv(circuitHalfOpenEnv, "4")

	got := toolCircuitConfigFromEnv()
	want := tooluse.CircuitBreakerConfig{
		FailureThreshold: 12, BaseCooldown: 45 * time.Second, MaxCooldown: 12 * time.Minute,
		BackoffMultiplier: 3.5, HalfOpenMaxAttempts: 4,
	}
	if got != want {
		t.Fatalf("AC③ 五项阈值未全部落到配置上：\n got %+v\nwant %+v", got, want)
	}
}

func TestToolCircuitConfigFromEnvRejectsBadValues(t *testing.T) {
	// 基线：默认配置（threshold=5 base=30s max=5m backoff=2 halfopen=1）
	t.Setenv(circuitThresholdEnv, "0")
	got := toolCircuitConfigFromEnv()
	if got.FailureThreshold != 5 {
		t.Fatalf("threshold=0（会让每个请求都被拒）不应被采纳，实际 %+v", got)
	}

	t.Setenv(circuitThresholdEnv, "100000")
	if got := toolCircuitConfigFromEnv(); got.FailureThreshold != 5 {
		t.Fatalf("越界 threshold 不应被采纳，实际 %+v", got)
	}

	t.Setenv(circuitThresholdEnv, "abc")
	if got := toolCircuitConfigFromEnv(); got.FailureThreshold != 5 {
		t.Fatalf("非整数 threshold 不应被采纳，实际 %+v", got)
	}

	// 常见事故一：想写 30 秒只写了 30 ⇒ ParseDuration 直接报错，回退默认 30s
	t.Setenv(circuitThresholdEnv, "8")
	t.Setenv(circuitBaseCooldownEnv, "30")
	if got := toolCircuitConfigFromEnv(); got.BaseCooldown != 30*time.Second {
		t.Fatalf("裸数字时长不应被采纳，实际 %s", got.BaseCooldown)
	}
	// 常见事故二：单位写错成纳秒（能解析但量级差 9 个数量级）⇒ 低于下限，回退默认 30s
	t.Setenv(circuitBaseCooldownEnv, "1ns")
	if got := toolCircuitConfigFromEnv(); got.BaseCooldown != 30*time.Second {
		t.Fatalf("亚毫秒冷却不应被采纳（等于没有熔断），实际 %s", got.BaseCooldown)
	}

	t.Setenv(circuitBaseCooldownEnv, "10m")
	t.Setenv(circuitMaxCooldownEnv, "5s")
	if got := toolCircuitConfigFromEnv(); got.MaxCooldown < got.BaseCooldown {
		t.Fatalf("max<base 时退避上限应被抬起（否则指数退避被反向夹住），实际 %+v", got)
	}

	t.Setenv(circuitMaxCooldownEnv, "")
	t.Setenv(circuitBackoffEnv, "0.5")
	if got := toolCircuitConfigFromEnv(); got.BackoffMultiplier != 2.0 {
		t.Fatalf("<1 的退避倍数不应被采纳，实际 %+v", got)
	}
}

func TestApplyToolCircuitBreakerOffLeavesExecutorUnchanged(t *testing.T) {
	restoreCircuitState(t)
	t.Setenv(circuitFlagEnv, "")

	config := tooluse.ToolExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		RateLimiter:    tooluse.NewTokenBucketLimiter(20, 50),
		// 预置一个 registry：off 分支必须把它清掉，否则"关旗 = 与接线前一致"只是注释里的承诺，
		// 而装配顺序若将来被挪到别处（先挂熔断再判旗），这里就是唯一的锁。
		CircuitBreaker:       tooluse.NewCircuitBreakerRegistry(tooluse.DefaultCircuitBreakerConfig()),
		CircuitBreakerShadow: true,
		OnCircuitDecision:    func(context.Context, tooluse.CircuitDecision) {},
	}
	before := config
	if mode := applyToolCircuitBreaker(&config); mode != toolCircuitOff {
		t.Fatalf("未配开关应为 off，实际 %s", mode)
	}
	if config.CircuitBreaker != nil {
		t.Error("off 时 CircuitBreaker 必须为 nil（非 nil 会让熔断进装饰链）")
	}
	if config.CircuitBreakerShadow || config.OnCircuitDecision != nil {
		t.Error("off 时 shadow 标志与判定回调都应清空")
	}
	if config.DefaultTimeout != before.DefaultTimeout || config.RateLimiter != before.RateLimiter {
		t.Error("apply 不应动无关字段")
	}
	if mode, _, registry, decisions := GetToolCircuitSnapshot(); mode != "off" || registry != nil || decisions != nil {
		t.Fatalf("off 时快照应整体为空：%s registry=%v decisions=%v", mode, registry, decisions)
	}

	tool := newCircuitProbeTool("off.tool")
	reg := tooluse.NewToolRegistry()
	_ = reg.Register(tool)
	exec := tooluse.NewToolExecutor(reg, config)
	for i := 0; i < 20; i++ {
		if res := exec.Execute(context.Background(), tooluse.ExecuteRequest{ToolName: "off.tool"}); errors.Is(res.Err, tooluse.ErrCircuitOpen) {
			t.Fatalf("关旗后第 %d 次调用被拦 ⇒ 熔断事实上仍生效", i+1)
		}
	}
	if tool.called() != 20 {
		t.Fatalf("关旗时 20 次调用应全部到达工具本体，实际 %d", tool.called())
	}
}

func TestApplyToolCircuitBreakerShadowAndEnforceFlags(t *testing.T) {
	cases := []struct {
		env         string
		wantMode    toolCircuitMode
		wantShadow  bool
		wantWired   bool
		wantReports bool
	}{
		{"shadow", toolCircuitShadow, true, true, true},
		{"enforce", toolCircuitEnforce, false, true, true},
		{"true", toolCircuitShadow, true, true, true},
		{"garbage", toolCircuitOff, false, false, false},
	}
	for _, tt := range cases {
		t.Run(tt.env, func(t *testing.T) {
			restoreCircuitState(t)
			t.Setenv(circuitFlagEnv, tt.env)
			t.Setenv(circuitThresholdEnv, "2")

			var config tooluse.ToolExecutorConfig
			mode := applyToolCircuitBreaker(&config)
			if mode != tt.wantMode {
				t.Fatalf("模式 = %s，期望 %s", mode, tt.wantMode)
			}
			if (config.CircuitBreaker != nil) != tt.wantWired {
				t.Fatalf("registry 是否挂载 = %v，期望 %v", config.CircuitBreaker != nil, tt.wantWired)
			}
			if config.CircuitBreakerShadow != tt.wantShadow {
				t.Fatalf("shadow 标志 = %v，期望 %v", config.CircuitBreakerShadow, tt.wantShadow)
			}
			if (config.OnCircuitDecision != nil) != tt.wantReports {
				t.Fatalf("判定回调 = %v，期望 %v", config.OnCircuitDecision != nil, tt.wantReports)
			}
			if tt.wantWired {
				if got := config.CircuitBreaker.Config().FailureThreshold; got != 2 {
					t.Fatalf("阈值未落到挂载的 registry 上：%d", got)
				}
			}
		})
	}
}

// TestToolCircuitShadowKeepsToolCallsIdentical AC②：走生产接线函数装配出的 executor，
// shadow 下的调用序列必须与"根本没接熔断"逐次一致。
func TestToolCircuitShadowKeepsToolCallsIdentical(t *testing.T) {
	const calls = 10

	// 对照组：关旗
	restoreCircuitState(t)
	t.Setenv(circuitFlagEnv, "off")
	t.Setenv(circuitThresholdEnv, "2")
	offConfig := tooluse.ToolExecutorConfig{}
	applyToolCircuitBreaker(&offConfig)
	offExec, offTool, name := newExecutorWithCircuit(t, offConfig)
	offResults := make([]string, 0, calls)
	for i := 0; i < calls; i++ {
		offResults = append(offResults, probeOutcome(offExec.Execute(context.Background(), tooluse.ExecuteRequest{ToolName: name})))
	}
	if offTool.called() != calls {
		t.Fatalf("对照组应有 %d 次到达下游，实际 %d", calls, offTool.called())
	}

	// 实验组：shadow，同一工具、同样次数
	restoreCircuitState(t)
	t.Setenv(circuitFlagEnv, "shadow")
	t.Setenv(circuitThresholdEnv, "2")
	shadowConfig := tooluse.ToolExecutorConfig{}
	if mode := applyToolCircuitBreaker(&shadowConfig); mode != toolCircuitShadow {
		t.Fatalf("模式 = %s，期望 shadow", mode)
	}
	shadowExec, shadowTool, name2 := newExecutorWithCircuit(t, shadowConfig)
	shadowResults := make([]string, 0, calls)
	blocked := 0
	for i := 0; i < calls; i++ {
		res := shadowExec.Execute(context.Background(), tooluse.ExecuteRequest{ToolName: name2})
		shadowResults = append(shadowResults, probeOutcome(res))
		if errors.Is(res.Err, tooluse.ErrCircuitOpen) {
			blocked++
		}
	}
	if blocked != 0 {
		t.Fatalf("shadow 下出现 %d 次 ErrCircuitOpen，放行行为已变", blocked)
	}
	if shadowTool.called() != calls {
		t.Errorf("shadow 下应 %d 次全部到达下游，实际 %d 次", calls, shadowTool.called())
	}
	for i := range offResults {
		if offResults[i] != shadowResults[i] {
			t.Fatalf("第 %d 次调用结果与关旗不一致：\n off=%s\n shadow=%s", i+1, offResults[i], shadowResults[i])
		}
	}
	// 前置：熔断确实打开过，否则"一致"是因为没接线
	if st := shadowConfig.CircuitBreaker.State(name2); st != tooluse.CircuitOpen {
		t.Fatalf("前置条件不成立：shadow 跑 %d 次后应为 open，实际 %s", calls, st)
	}
	// 判定报告必须已把"本会被拦"的次数记下来（AC①的另一半：可统计）
	_, _, _, decisions := GetToolCircuitSnapshot()
	if decisions == nil {
		t.Fatal("shadow 下判定累计器不应为 nil")
	}
	rep := decisions.Report()
	if rep.Total != calls || rep.WouldBlock != calls-2 {
		t.Fatalf("判定报告错：%+v（阈值 2 ⇒ 期望 total=%d would_block=%d）", rep, calls, calls-2)
	}
}

// TestToolCircuitEnforceBlocksAfterThreshold 反向对照：enforce 真的会拦。
// 缺了它，上一条测试可能被"shadow 分支根本没进熔断器"蒙混过关。
func TestToolCircuitEnforceBlocksAfterThreshold(t *testing.T) {
	const calls = 10
	restoreCircuitState(t)
	t.Setenv(circuitFlagEnv, "enforce")
	t.Setenv(circuitThresholdEnv, "2")

	config := tooluse.ToolExecutorConfig{}
	if mode := applyToolCircuitBreaker(&config); mode != toolCircuitEnforce {
		t.Fatalf("模式 = %s，期望 enforce", mode)
	}
	exec, tool, name := newExecutorWithCircuit(t, config)
	blocked := 0
	for i := 0; i < calls; i++ {
		res := exec.Execute(context.Background(), tooluse.ExecuteRequest{ToolName: name})
		if errors.Is(res.Err, tooluse.ErrCircuitOpen) {
			blocked++
		}
	}
	if blocked == 0 {
		t.Fatal("enforce 下一次都没拦 ⇒ 熔断未生效，shadow 对照失效")
	}
	if tool.called() >= calls {
		t.Errorf("enforce 下应有调用被拦下，实际全部到达下游（%d 次）", tool.called())
	}
	if _, _, _, decisions := GetToolCircuitSnapshot(); decisions.Report().WouldBlock == 0 {
		t.Error("生效态也必须产出判定报告（转阻断后仍要能看见拦了多少）")
	}
}

// TestToolCircuitDecisionLandsInAuditLog AC①：连续失败后**日志里**出现熔断判定记录。
//
// 抓日志要做两件事才对：换 os.Stdout 之外还必须重建全局日志器——GetLogger 会把实例
// 连同当时的 os.Stdout 一起缓存，只换 stdout 抓不到任何东西（这条最容易写成假绿）。
func TestToolCircuitDecisionLandsInAuditLog(t *testing.T) {
	restoreCircuitState(t)
	t.Setenv(circuitFlagEnv, "shadow")
	t.Setenv(circuitThresholdEnv, "2")

	oldOut := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("管道创建失败：%v", err)
	}
	os.Stdout = w
	logger.InitLogger(logger.LoggingConfig{Level: "info", Format: "json", Output: "stdout"})
	t.Cleanup(func() {
		os.Stdout = oldOut
		logger.InitLogger(logger.DefaultConfig())
	})

	config := tooluse.ToolExecutorConfig{}
	if mode := applyToolCircuitBreaker(&config); mode != toolCircuitShadow {
		t.Fatalf("模式 = %s，期望 shadow", mode)
	}
	exec, _, name := newExecutorWithCircuit(t, config)
	for i := 0; i < 5; i++ {
		exec.Execute(context.Background(), tooluse.ExecuteRequest{ToolName: name})
	}

	if err := w.Close(); err != nil {
		t.Fatalf("关闭写端失败：%v", err)
	}
	os.Stdout = oldOut
	captured, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatalf("读取日志失败：%v", readErr)
	}
	logged := string(captured)
	t.Logf("捕获到的审计日志：\n%s", logged)

	if !strings.Contains(logged, `"event":"tool_circuit_decision"`) {
		t.Fatal("日志里没有熔断判定记录（AC① 未兑现）")
	}
	if !strings.Contains(logged, `"shadow":true`) {
		t.Error("shadow 态的判定记录应标 shadow=true")
	}
	if !strings.Contains(logged, `"would_block":true`) {
		t.Error("阈值 2、调用 5 次 ⇒ 应有 would_block=true 的记录")
	}
	if strings.Contains(logged, `"would_block":false`) {
		t.Error("放行判定不该产生日志（每次工具调用两行且无信息量）")
	}
	// 日志与计数必须同源：同一次跑出来的数字要能对上
	_, _, _, decisions := GetToolCircuitSnapshot()
	if decisions == nil {
		t.Fatal("shadow 下判定累计器不应为 nil")
	}
	rep := decisions.Report()
	var loggedDecisions int
	for _, line := range strings.Split(logged, "\n") {
		if !strings.Contains(line, `"event":"tool_circuit_decision"`) {
			continue
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(line), &parsed); err != nil {
			t.Fatalf("审计日志不是合法 JSON（下游采集会挂）：%v\n%s", err, line)
		}
		if parsed["tool_name"] != name {
			t.Errorf("判定记录缺工具名：%v", parsed["tool_name"])
		}
		loggedDecisions++
	}
	if loggedDecisions != int(rep.WouldBlock) {
		t.Errorf("日志条数 %d 与计数 would_block=%d 不一致", loggedDecisions, rep.WouldBlock)
	}
}

func probeOutcome(res tooluse.ExecuteResult) string {
	errText := "<nil>"
	if res.Err != nil {
		errText = res.Err.Error()
	}
	return errText + "|" + res.ToolResult.Error + "|" + boolStr(res.Success)
}

func boolStr(b bool) string {
	if b {
		return "ok"
	}
	return "fail"
}

// TestObserveToolCircuitDecisionNilCounterSafe 判定回调在计数器缺失时不得 panic
// （observeToolCircuitDecision 会被挂进每一次工具调用的装饰链里）。
func TestObserveToolCircuitDecisionNilCounterSafe(t *testing.T) {
	restoreCircuitState(t)
	circuitDecisions = nil
	func() {
		defer func() {
			if p := recover(); p != nil {
				t.Fatalf("判定回调 panic：%v", p)
			}
		}()
		observeToolCircuitDecision(context.Background(), tooluse.CircuitDecision{ToolName: "x", WouldBlock: true, Shadow: true})
		observeToolCircuitDecision(context.Background(), tooluse.CircuitDecision{ToolName: "x", WouldBlock: false})
	}()
}

// ── 参数中心这一层（agent_tool 组 circuit_* 五项）─────────────────────────
//
// 上面那批用例只覆盖 env 一层。下面这组覆盖"库里改值 → 装配读到"这条腿，
// 断言的是 resolveToolCircuitConfig 那条唯一的优先级链：代码兜底 < 参数中心 < 环境变量。
//
// 这一层最容易写成的三种假绿，各由一条用例盯着：
//   ① 写了参数读取函数却没人调用它          → 装配腿（apply → registry.Config()）
//   ② 种子值与代码兜底不等价（接线即变更）  → 灌完种子后 resolve 必须逐字等于默认
//   ③ 只测了"读得到"，没测"越界读不进来"    → 直写库绕过写侧校验那一格

// clearCircuitEnv 把五项 env 全部置空：让"只剩参数中心这一层"成为可断言的前提，
// 而不是碰运气依赖上一测试跑完后环境里剩了什么。
func clearCircuitEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		circuitThresholdEnv, circuitBaseCooldownEnv, circuitMaxCooldownEnv,
		circuitBackoffEnv, circuitHalfOpenEnv,
	} {
		t.Setenv(k, "")
	}
}

// newCircuitParamFixture 建测试库、灌种子，并把参数中心换成"真读库"的实例。
//
// 同时返回 *gorm.DB 是为了"绕过写侧 validateValue 直接改库"那一格：
// 读取侧的区间夹逼如果只能靠页面写入来触发，那它其实从来没被这条判据看过
// （Min/Max 会先把越界值挡在接口上）。
//
// t.Cleanup 还原全局：globalConfigParam 是进程级包级变量，留着带 DB 的实例
// 会把同包后续用例的 Get* 一并带走。
func newCircuitParamFixture(t *testing.T) (*gorm.DB, *service.ConfigParamService) {
	t.Helper()
	db := testutil.NewTestDBOrSkip(t, &model.ConfigParam{}, &model.ConfigParamAuditLog{})
	if db == nil {
		t.Skip("no DB")
	}
	if err := service.SeedConfigParams(context.Background(), db); err != nil {
		t.Fatalf("SeedConfigParams 失败：%v", err)
	}
	svc := service.NewConfigParamService(db)
	service.SetGlobalForTest(svc)
	t.Cleanup(func() { service.SetGlobalForTest(nil) })
	return db, svc
}

func TestResolveToolCircuitConfigWithoutDBMatchesEnvOnly(t *testing.T) {
	clearCircuitEnv(t)
	service.SetGlobalForTest(nil)
	t.Cleanup(func() { service.SetGlobalForTest(nil) })

	// 无库 ⇒ 参数层必须逐条回落代码兜底，而不是返回零值配置
	// （零值 threshold=0 会让"每个请求都被拒"，比不设配置危险得多）。
	if got, want := resolveToolCircuitConfig(), tooluse.DefaultCircuitBreakerConfig(); got != want {
		t.Fatalf("无库时 resolve 必须逐字等于代码兜底：\n got %+v\nwant %+v", got, want)
	}

	// 配了 env 时，无库的 resolve 与"只看 env"的读法必须一致（参数层不许凭空造值）。
	t.Setenv(circuitThresholdEnv, "7")
	t.Setenv(circuitBackoffEnv, "4")
	if got, want := resolveToolCircuitConfig(), toolCircuitConfigFromEnv(); got != want {
		t.Errorf("无库时两层结果不一致：resolve=%+v envOnly=%+v", got, want)
	}
	if got := resolveToolCircuitConfig().FailureThreshold; got != 7 {
		t.Errorf("无库时 env 仍应生效，threshold=%d", got)
	}
}

func TestCircuitSeedDefaultsEqualCodeDefaults(t *testing.T) {
	_, svc := newCircuitParamFixture(t)
	clearCircuitEnv(t)
	ctx := context.Background()

	// 五条必须真的落库了。用"绝不可能的兜底值"探：行不存在时 Get* 返回兜底，
	// 行存在时返回库里的合法值——同一个调用能同时判"接错键名"和"seed 没插成功"。
	probes := []struct {
		key  string
		read func(string) int64
	}{
		{"circuit_failure_threshold", func(k string) int64 { return int64(svc.GetInt(ctx, "agent_tool", k, -1)) }},
		{"circuit_base_cooldown", func(k string) int64 { return int64(svc.GetDuration(ctx, "agent_tool", k, -time.Second).Seconds()) }},
		{"circuit_max_cooldown", func(k string) int64 { return int64(svc.GetDuration(ctx, "agent_tool", k, -time.Second).Seconds()) }},
		{"circuit_backoff_multiplier", func(k string) int64 { return int64(svc.GetFloat(ctx, "agent_tool", k, -1)) }},
		{"circuit_half_open_max_attempts", func(k string) int64 { return int64(svc.GetInt(ctx, "agent_tool", k, -1)) }},
	}
	for _, p := range probes {
		if got := p.read(p.key); got <= 0 {
			t.Errorf("agent_tool.%s 没落库（读到探针兜底 %d）：接了线但库里没这行 = 僵尸键，页面上看不见", p.key, got)
		}
	}

	// 关键不变量：种子默认值 == tooluse 代码兜底 ⇒ "接上参数中心但没人改过"是零行为变更。
	// 这条一旦红，这次改动本身就成了一次没打招呼的灰度。
	if got, want := resolveToolCircuitConfig(), tooluse.DefaultCircuitBreakerConfig(); got != want {
		t.Errorf("种子默认值与代码兜底不等价：\n got %+v\nwant %+v", got, want)
	}
}

func TestResolveToolCircuitConfigReadsParamsIntoRegistry(t *testing.T) {
	_, svc := newCircuitParamFixture(t)
	clearCircuitEnv(t)
	ctx := context.Background()

	want := tooluse.CircuitBreakerConfig{
		FailureThreshold: 9, BaseCooldown: 45 * time.Second, MaxCooldown: 900 * time.Second,
		BackoffMultiplier: 2.5, HalfOpenMaxAttempts: 3,
	}
	for _, item := range []struct{ key, value string }{
		{"circuit_failure_threshold", "9"},
		{"circuit_base_cooldown", "45"},
		{"circuit_max_cooldown", "900"},
		{"circuit_backoff_multiplier", "2.5"},
		{"circuit_half_open_max_attempts", "3"},
	} {
		if err := svc.UpdateValue(ctx, "agent_tool", item.key, item.value, 1); err != nil {
			t.Fatalf("改 agent_tool.%s=%s 失败：%v", item.key, item.value, err)
		}
	}

	if got := resolveToolCircuitConfig(); got != want {
		t.Fatalf("参数中心的五项没被读走：\n got %+v\nwant %+v", got, want)
	}

	// 装配腿：只测 resolve 不够——函数写对了但 apply 仍调 env 那份，这条腿照样是死的。
	restoreCircuitState(t)
	t.Setenv(circuitFlagEnv, "shadow")
	var config tooluse.ToolExecutorConfig
	if mode := applyToolCircuitBreaker(&config); mode != toolCircuitShadow {
		t.Fatalf("模式 = %s，期望 shadow", mode)
	}
	if got := config.CircuitBreaker.Config(); got != want {
		t.Errorf("挂到 executor 上的 registry 带的不是参数中心的配置：\n got %+v\nwant %+v", got, want)
	}
	if _, applied, _, _ := GetToolCircuitSnapshot(); applied != want {
		t.Errorf("调试快照读到的不是生效配置：%+v", applied)
	}
}

func TestResolveToolCircuitConfigEnvBeatsParamsPerItem(t *testing.T) {
	_, svc := newCircuitParamFixture(t)
	clearCircuitEnv(t)
	ctx := context.Background()
	for _, item := range []struct{ key, value string }{
		{"circuit_failure_threshold", "9"},
		{"circuit_half_open_max_attempts", "3"},
	} {
		if err := svc.UpdateValue(ctx, "agent_tool", item.key, item.value, 1); err != nil {
			t.Fatalf("改 agent_tool.%s=%s 失败：%v", item.key, item.value, err)
		}
	}

	// env 只压 threshold 一项：其余项必须仍走参数中心（逐层合并，不是整层覆盖）。
	t.Setenv(circuitThresholdEnv, "11")
	got := resolveToolCircuitConfig()
	if got.FailureThreshold != 11 {
		t.Errorf("env 未压过参数中心：threshold=%d", got.FailureThreshold)
	}
	if got.HalfOpenMaxAttempts != 3 {
		t.Errorf("env 覆盖一项时把参数中心其余项带走了：%+v", got)
	}

	// env 越界时回落对象是"下层"（参数中心的 9），不是"代码默认 5"：
	// 否则运维在库里配的合法值会因为一个打错的临时 env 一起丢掉。
	t.Setenv(circuitThresholdEnv, "0")
	if got := resolveToolCircuitConfig(); got.FailureThreshold != 9 {
		t.Errorf("env 越界应回落参数中心的 9，实际 %d", got.FailureThreshold)
	}

	t.Setenv(circuitThresholdEnv, "")
	if got := resolveToolCircuitConfig(); got.FailureThreshold != 9 {
		t.Errorf("撤掉 env 后应回到参数中心的 9，实际 %d", got.FailureThreshold)
	}
}

func TestResolveToolCircuitConfigClampsParamLayer(t *testing.T) {
	_, svc := newCircuitParamFixture(t)
	clearCircuitEnv(t)

	// base 抬到 600s，max 留在种子默认的 300s ⇒ 上限必须被抬到 base。
	// 不夹的话 calculateCooldown 里 `cooldown > MaxCooldown 就取 MaxCooldown`，
	// 指数退避会被反向夹住（第 2 次起算出的 600s 又掉回 300s）。
	if err := svc.UpdateValue(context.Background(), "agent_tool", "circuit_base_cooldown", "600", 1); err != nil {
		t.Fatalf("改 circuit_base_cooldown 失败：%v", err)
	}
	got := resolveToolCircuitConfig()
	if got.BaseCooldown != 600*time.Second {
		t.Fatalf("前置不成立：base 未读到 600s，实际 %s", got.BaseCooldown)
	}
	if got.MaxCooldown != 600*time.Second {
		t.Errorf("max<base 时退避上限应被抬到 base，实际 max=%s base=%s", got.MaxCooldown, got.BaseCooldown)
	}
}

func TestCircuitParamOverRejectsOutOfRangeFromDB(t *testing.T) {
	db, _ := newCircuitParamFixture(t)
	clearCircuitEnv(t)

	// 直写库、绕开页面写侧的 Min/Max：validateValue 只拦"从接口改"这一条路。
	// 读取侧不重复夹一遍的话，threshold=0 会原样装进 registry ⇒ 每个工具调用都被拒。
	for _, item := range []struct{ key, value string }{
		{"circuit_failure_threshold", "0"},
		{"circuit_half_open_max_attempts", "1001"},
		{"circuit_base_cooldown", "999999"},
	} {
		if err := db.Table("config_params").
			Where("param_group = ? AND key = ?", "agent_tool", item.key).
			Update("param_value", item.value).Error; err != nil {
			t.Fatalf("直写 agent_tool.%s=%s 失败：%v", item.key, item.value, err)
		}
	}

	base := tooluse.DefaultCircuitBreakerConfig()
	got := circuitParamOver(base)
	if got.FailureThreshold != base.FailureThreshold {
		t.Errorf("threshold=0（会让每个请求都被拒）不该被采纳，实际 %+v", got)
	}
	if got.HalfOpenMaxAttempts != base.HalfOpenMaxAttempts {
		t.Errorf("越界 half_open 配额不该被采纳，实际 %+v", got)
	}
	if got.BaseCooldown != base.BaseCooldown {
		t.Errorf("越界 base_cooldown 不该被采纳，实际 %+v", got)
	}
	// 逐项独立：三条坏值只丢这三条，第四项仍是库里的合法值（种子 300s = 5m）。
	if got.MaxCooldown != 5*time.Minute {
		t.Errorf("其余项被坏值带走了：max_cooldown=%s，期望种子值 5m", got.MaxCooldown)
	}
	// 且这里的回落必须是"下层值"而不是零值配置。
	if got == (tooluse.CircuitBreakerConfig{}) {
		t.Error("越界时整份配置被清零了")
	}
}
