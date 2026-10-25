package service

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"hivemtk-user/internal/cache"
)

// 空旅程（窗口内零进入）的漏斗必须仍然可 JSON 序列化。
//
// 起因（实测）：把 FF_LTC_ORDER_DRAFT_DB 打开后，草稿竖的旅程服务被装配上，
// GET /api/sales-workbench/team-dashboard 立刻变成 HTTP 200 + **0 字节**。
// 根因是 Funnel() 里 StageRate = Customers/TotalEntered*100 没有守卫：
// TotalEntered=0 时是 0/0 = NaN，json.Marshal 对 NaN 直接报
// "unsupported value: NaN"；而 gin 的 c.JSON 是「先写 200 状态码、再序列化」，
// 序列化失败就只剩一个空 body —— 客户端看到的是一次成功的空响应。
//
// 判据是「能序列化」而不是「等于某个数」：同函数里 EndToEndRate / AvgDwellDays /
// StepRate 都已有 >0 守卫，只有 StageRate 漏了，所以用序列化把这条腿钉住。
func TestFunnel_EmptyJourneyIsJSONMarshalable(t *testing.T) {
	svc := NewCustomerJourneyServiceWithCache(cache.NewMemoryCache())

	funnel := svc.Funnel(context.Background())
	if funnel == nil {
		t.Fatal("Funnel() 返回 nil")
	}
	if funnel.TotalEntered != 0 {
		t.Fatalf("前置不成立：期望空旅程 TotalEntered=0，实际 %d", funnel.TotalEntered)
	}
	if len(funnel.Stages) == 0 {
		t.Fatal("前置不成立：漏斗阶段集合为空，后续断言没有对象")
	}

	for _, st := range funnel.Stages {
		if math.IsNaN(st.StageRate) || math.IsInf(st.StageRate, 0) {
			t.Errorf("阶段 %s 的 StageRate 非有限值：%v", st.Stage, st.StageRate)
		}
		if math.IsNaN(st.StepRate) || math.IsInf(st.StepRate, 0) {
			t.Errorf("阶段 %s 的 StepRate 非有限值：%v", st.Stage, st.StepRate)
		}
	}
	if math.IsNaN(funnel.EndToEndRate) || math.IsInf(funnel.EndToEndRate, 0) {
		t.Errorf("EndToEndRate 非有限值：%v", funnel.EndToEndRate)
	}

	raw, err := json.Marshal(funnel)
	if err != nil {
		t.Fatalf("空旅程漏斗不可序列化（gin 会因此回 200 + 空 body）：%v", err)
	}
	if len(raw) == 0 {
		t.Fatal("序列化结果为空")
	}
}
