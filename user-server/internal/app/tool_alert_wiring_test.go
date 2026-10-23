package app

import (
	"sync"
	"testing"

	"hivemtk-user/internal/aiagent/agent/tooluse"
)

// E-6a 验收用例（规划 §4.6）：熔断打开 ⇒ 告警器被调用；shadow 态只记录不告警。
//
// observeToolCircuitDecision 是生产装配的观察者：WouldBlock 且非 shadow 时
// 必须调 AlertCircuitOpen 恰好一次；shadow 判定（WouldBlock=true、Shadow=true）
// 不得触发告警——那是"只记录不告警"口径的验收点。
func TestObserveToolCircuitDecision_AlertsOnBlockNotShadow(t *testing.T) {
	prev := toolAlertManager
	t.Cleanup(func() { toolAlertManager = prev })

	var mu sync.Mutex
	var alerts []tooluse.AlertEvent
	mgr := tooluse.NewToolAlertManager()
	mgr.AddHandler(tooluse.AlertHandlerFunc(func(e tooluse.AlertEvent) {
		mu.Lock()
		defer mu.Unlock()
		alerts = append(alerts, e)
	}))
	toolAlertManager = mgr

	// ① enforce 态 would_block ⇒ 告警 1 条
	observeToolCircuitDecision(t.Context(), tooluse.CircuitDecision{
		ToolName: "search_kb", State: tooluse.CircuitOpen, WouldBlock: true, Shadow: false,
	})
	mu.Lock()
	if len(alerts) != 1 {
		t.Fatalf("enforce would_block 应恰好 1 条告警，实际 %d", len(alerts))
	}
	if alerts[0].ToolName != "search_kb" {
		t.Fatalf("告警应点名工具名，实际 %q", alerts[0].ToolName)
	}
	mu.Unlock()

	// ② shadow 判定 ⇒ 只记录不告警（计数不变）
	observeToolCircuitDecision(t.Context(), tooluse.CircuitDecision{
		ToolName: "search_kb", State: tooluse.CircuitOpen, WouldBlock: true, Shadow: true,
	})
	mu.Lock()
	defer mu.Unlock()
	if len(alerts) != 1 {
		t.Fatalf("shadow 态不得触发告警（只记录），实际告警数 %d", len(alerts))
	}
}

// ③ 放行判定（WouldBlock=false）不告警——绝大多数调用没有信息量。
func TestObserveToolCircuitDecision_PassDoesNotAlert(t *testing.T) {
	prev := toolAlertManager
	t.Cleanup(func() { toolAlertManager = prev })

	var alerts []tooluse.AlertEvent
	mgr := tooluse.NewToolAlertManager()
	mgr.AddHandler(tooluse.AlertHandlerFunc(func(e tooluse.AlertEvent) { alerts = append(alerts, e) }))
	toolAlertManager = mgr

	observeToolCircuitDecision(t.Context(), tooluse.CircuitDecision{
		ToolName: "search_kb", State: tooluse.CircuitClosed, WouldBlock: false, Shadow: false,
	})
	if len(alerts) != 0 {
		t.Fatalf("放行判定不得告警，实际 %d 条", len(alerts))
	}
}

// ④ off 态（toolAlertManager=nil）判定不 panic——装配降级不炸主链。
func TestObserveToolCircuitDecision_NilManagerNoPanic(t *testing.T) {
	prev := toolAlertManager
	t.Cleanup(func() { toolAlertManager = prev })
	toolAlertManager = nil
	observeToolCircuitDecision(t.Context(), tooluse.CircuitDecision{
		ToolName: "search_kb", State: tooluse.CircuitOpen, WouldBlock: true, Shadow: false,
	})
}
