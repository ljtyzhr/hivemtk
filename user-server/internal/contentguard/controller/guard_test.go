// guard_test.go 管理端 HTTP 出口的冒烟测试（内存 Store 装配，不连库）。
//
// 覆盖点：
//   - 每条路由的**形状**（响应契约 {code,data}、分页 {list,total}）；
//   - sentinel error → 业务码的映射（404 记录不存在、4000 参数非法、5000 未装配）；
//   - "页面显示拦 / 线上真的拦"同源：/scan 与 worker 走的是同一套引擎。
package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"hivemtk-user/internal/contentguard/memory"
	"hivemtk-user/internal/contentguard/model"
	"hivemtk-user/internal/contentguard/service"
	bizerr "hivemtk-user/internal/domain/errors"
)

type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type testEnv struct {
	router *gin.Engine
	rt     *service.Runtime
	store  *memory.Store
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store := memory.New()
	deps := store.Deps()
	deps.Config = service.PipelineConfig{BatchSize: 16}
	rt := service.NewRuntime(deps)
	r := gin.New()
	NewGuardController(rt).RegisterRoutes(r.Group("/api"))
	return &testEnv{router: r, rt: rt, store: store}
}

func (e *testEnv) do(t *testing.T, method, path string, body any) envelope {
	t.Helper()
	var buf *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("序列化请求体失败：%v", err)
		}
		buf = bytes.NewReader(raw)
	} else {
		buf = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)

	// 本仓约定：业务错误也回 HTTP 200，错误由 code 字段表达。
	if w.Code != http.StatusOK {
		t.Fatalf("%s %s 状态码 = %d，期望 200，body=%s", method, path, w.Code, w.Body.String())
	}
	var env envelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("响应不是 {code,data} 结构：%v body=%s", err, w.Body.String())
	}
	return env
}

func (e *testEnv) mustOK(t *testing.T, method, path string, body any) envelope {
	t.Helper()
	env := e.do(t, method, path, body)
	if env.Code != bizerr.CodeSuccess {
		t.Fatalf("%s %s 业务码 = %d（%s），期望 0", method, path, env.Code, env.Message)
	}
	return env
}

func seedDeny(t *testing.T, e *testEnv, word string, sev int) {
	t.Helper()
	env := e.mustOK(t, http.MethodPost, "/api/manage/content-guard/words", map[string]any{
		"list_kind": model.ListKindDeny, "word": word, "severity": sev, "category": "test",
	})
	var saved model.GuardWord
	if err := json.Unmarshal(env.Data, &saved); err != nil {
		t.Fatalf("解析词条失败：%v", err)
	}
	if saved.ID == 0 || !saved.Enabled {
		t.Fatalf("词条未正确落库：%+v", saved)
	}
}

// ── 用例 ───────────────────────────────────────────────────────────────

func TestScan_ReturnsVerdict(t *testing.T) {
	e := newTestEnv(t)
	seedDeny(t, e, "毒品", 3)

	env := e.mustOK(t, http.MethodPost, "/api/manage/content-guard/scan", map[string]any{
		"content": "我要买 毒 品",
	})
	var got struct {
		Verdict     string           `json:"verdict"`
		Action      string           `json:"action"`
		FreezeLevel string           `json:"freeze_level"`
		MaxSeverity int              `json:"max_severity"`
		Hits        []model.GuardHit `json:"hits"`
		Sources     []string         `json:"sources"`
	}
	if err := json.Unmarshal(env.Data, &got); err != nil {
		t.Fatalf("解析试算结果失败：%v", err)
	}
	if got.Verdict != model.VerdictBlock || got.Action != model.ActionReject {
		t.Fatalf("试算结论 = {%s,%s}，期望 block/reject", got.Verdict, got.Action)
	}
	if got.FreezeLevel != model.SanctionLevelTemp {
		t.Fatalf("首次拦截应给临时冻结档，得 %q", got.FreezeLevel)
	}
	if got.MaxSeverity != 3 || len(got.Hits) != 1 {
		t.Fatalf("证据不对：%+v", got)
	}
	// 试算不落任何记录、不入队：运营"测试一下"不能污染真实队列。
	if n := len(e.store.Records()); n != 0 {
		t.Fatalf("试算不得写记录，得 %d 条", n)
	}
	if n := len(e.store.Tasks()); n != 0 {
		t.Fatalf("试算不得入队，得 %d 条", n)
	}
}

func TestScan_DefaultsSubjectAndScene(t *testing.T) {
	e := newTestEnv(t)
	// 不传 subject_type / scene 时应按 user + inbound_message 试算（而不是报参数错误）。
	env := e.mustOK(t, http.MethodPost, "/api/manage/content-guard/scan", map[string]any{
		"content": "普通内容",
	})
	var got struct {
		Verdict string `json:"verdict"`
	}
	if err := json.Unmarshal(env.Data, &got); err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if got.Verdict != model.VerdictPass {
		t.Fatalf("无误内容应 pass，得 %s", got.Verdict)
	}
}

func TestWords_CRUDAndPaginationShape(t *testing.T) {
	e := newTestEnv(t)
	seedDeny(t, e, "毒品", 3)

	env := e.mustOK(t, http.MethodGet, "/api/manage/content-guard/words?page=1&page_size=10", nil)
	var list struct {
		List  []model.GuardWord `json:"list"`
		Total int64             `json:"total"`
	}
	if err := json.Unmarshal(env.Data, &list); err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if list.Total != 1 || len(list.List) != 1 {
		t.Fatalf("词库列表 = %d/%d，期望 1", list.Total, len(list.List))
	}

	// 同名词条幂等（不产生第二行）。
	seedDeny(t, e, "毒品", 3)
	if got := len(e.store.Words()); got != 1 {
		t.Fatalf("同名词条应幂等，得 %d 条", got)
	}

	e.mustOK(t, http.MethodDelete, "/api/manage/content-guard/words/1", nil)
	if got := len(e.store.Words()); got != 0 {
		t.Fatalf("删除后应为空，得 %d 条", got)
	}

	// 空词条必须被参数校验拦下（4000），不是 500。
	env = e.do(t, http.MethodPost, "/api/manage/content-guard/words", map[string]any{"word": "  "})
	if env.Code != bizerr.CodeParamInvalid {
		t.Fatalf("空词条业务码 = %d，期望 %d", env.Code, bizerr.CodeParamInvalid)
	}
	// 非法词条类型同样拦下。
	env = e.do(t, http.MethodPost, "/api/manage/content-guard/words", map[string]any{
		"word": "x", "list_kind": "ban",
	})
	if env.Code != bizerr.CodeParamInvalid {
		t.Fatalf("非法 list_kind 业务码 = %d，期望 %d", env.Code, bizerr.CodeParamInvalid)
	}
}

func TestWords_Import(t *testing.T) {
	e := newTestEnv(t)
	env := e.mustOK(t, http.MethodPost, "/api/manage/content-guard/words/import", map[string]any{
		"words": []map[string]any{
			{"word": "词一", "severity": 1},
			{"word": "词二", "severity": 2},
			{"word": "词一", "severity": 3}, // 同批次重复 ⇒ 去重
		},
	})
	var got struct {
		Imported int `json:"imported"`
	}
	if err := json.Unmarshal(env.Data, &got); err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if got.Imported != 2 {
		t.Fatalf("导入条数 = %d，期望 2（同批次重复应去重）", got.Imported)
	}

	// 导入后立刻生效（写入口失效词库快照）。
	scan := e.mustOK(t, http.MethodPost, "/api/manage/content-guard/scan", map[string]any{
		"subject_type": model.SubjectMerchant,
		"scene":        model.SceneMarketingCopy,
		"content":      "这里有词二",
	})
	var verdict struct {
		Verdict string `json:"verdict"`
	}
	if err := json.Unmarshal(scan.Data, &verdict); err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if verdict.Verdict != model.VerdictBlock {
		t.Fatalf("导入后应立刻生效（merchant sev2 拦），得 %s", verdict.Verdict)
	}

	env = e.do(t, http.MethodPost, "/api/manage/content-guard/words/import", map[string]any{"words": []any{}})
	if env.Code != bizerr.CodeParamInvalid {
		t.Fatalf("空 words 业务码 = %d，期望 %d", env.Code, bizerr.CodeParamInvalid)
	}
}

func TestPolicies_ListAndSave(t *testing.T) {
	e := newTestEnv(t)

	env := e.mustOK(t, http.MethodGet, "/api/manage/content-guard/policies", nil)
	var list struct {
		List  []model.GuardPolicy `json:"list"`
		Total int                 `json:"total"`
	}
	if err := json.Unmarshal(env.Data, &list); err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	// 库里没有策略行时也必须列出两条出厂策略（读不到 ≠ 没有策略）。
	if list.Total != 2 {
		t.Fatalf("策略数 = %d，期望 2（user + merchant 出厂策略）", list.Total)
	}
	var haveUser, haveMerchant bool
	for _, p := range list.List {
		switch p.SubjectType {
		case model.SubjectUser:
			haveUser = p.BlockSeverity == 3 && p.BlockAction == model.ActionReject
		case model.SubjectMerchant:
			haveMerchant = p.BlockSeverity == 2 && p.BlockAction == model.ActionBlockPublish
		}
	}
	if !haveUser || !haveMerchant {
		t.Fatalf("出厂策略口径不对：user=%v merchant=%v", haveUser, haveMerchant)
	}

	env = e.mustOK(t, http.MethodPut, "/api/manage/content-guard/policies", map[string]any{
		"subject_type": model.SubjectUser, "api_sample_rate": 30, "temp_freeze_minutes": 120,
	})
	var saved model.GuardPolicy
	if err := json.Unmarshal(env.Data, &saved); err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if saved.APISampleRate != 30 || saved.TempFreezeMinutes != 120 {
		t.Fatalf("保存结果 = %+v", saved)
	}
	if saved.BlockSeverity != 3 {
		t.Fatalf("未提交的字段应保留原值，block_severity = %d", saved.BlockSeverity)
	}

	env = e.do(t, http.MethodPut, "/api/manage/content-guard/policies", map[string]any{
		"subject_type": "tenant",
	})
	if env.Code != bizerr.CodeParamInvalid {
		t.Fatalf("非法主体类型业务码 = %d，期望 %d", env.Code, bizerr.CodeParamInvalid)
	}
	env = e.do(t, http.MethodPut, "/api/manage/content-guard/policies", map[string]any{
		"subject_type": model.SubjectUser, "scenes": "chat",
	})
	if env.Code != bizerr.CodeParamInvalid {
		t.Fatalf("非法场景业务码 = %d，期望 %d", env.Code, bizerr.CodeParamInvalid)
	}
}

// submit 走服务层入队并同步驱动一轮处理（等价 worker 的一轮 drain）。
func submit(t *testing.T, e *testEnv, content string) {
	t.Helper()
	if _, err := e.rt.SubmitReview(t.Context(), service.ScanRequest{
		SubjectType: model.SubjectUser, SubjectID: "u_1",
		Scene: model.SceneInboundMessage, Content: content,
	}); err != nil {
		t.Fatalf("提交审查失败：%v", err)
	}
	if _, err := e.rt.Pipeline.ProcessDue(t.Context(), 16); err != nil {
		t.Fatalf("处理审查失败：%v", err)
	}
}

func TestRecords_ListGetAndReview(t *testing.T) {
	e := newTestEnv(t)
	seedDeny(t, e, "毒品", 3)
	submit(t, e, "我要毒品")

	env := e.mustOK(t, http.MethodGet, "/api/manage/content-guard/records?subject_type=user", nil)
	var list struct {
		List  []model.GuardRecord `json:"list"`
		Total int64               `json:"total"`
	}
	if err := json.Unmarshal(env.Data, &list); err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if list.Total != 1 || len(list.List) != 1 {
		t.Fatalf("记录数 = %d/%d，期望 1", list.Total, len(list.List))
	}
	rec := list.List[0]
	if rec.Verdict != model.VerdictBlock || rec.SanctionID == 0 {
		t.Fatalf("记录 = %+v，期望 block 且已关联处置", rec)
	}

	env = e.mustOK(t, http.MethodGet, "/api/manage/content-guard/records/1", nil)
	var detail model.GuardRecord
	if err := json.Unmarshal(env.Data, &detail); err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if detail.ID != rec.ID {
		t.Fatalf("详情 ID = %d，期望 %d", detail.ID, rec.ID)
	}

	env = e.mustOK(t, http.MethodPost, "/api/manage/content-guard/records/1/review",
		map[string]any{"status": model.ManualStatusOverturned})
	var reviewed model.GuardRecord
	if err := json.Unmarshal(env.Data, &reviewed); err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if reviewed.ManualStatus != model.ManualStatusOverturned {
		t.Fatalf("复核状态 = %s", reviewed.ManualStatus)
	}
	// 改判必须连带解除处置。
	if _, ok := e.rt.IsBlocked(t.Context(), model.SubjectUser, "u_1"); ok {
		t.Fatal("改判后仍处于冻结状态")
	}

	env = e.do(t, http.MethodPost, "/api/manage/content-guard/records/1/review", map[string]any{"status": "maybe"})
	if env.Code != bizerr.CodeParamInvalid {
		t.Fatalf("非法复核状态业务码 = %d，期望 %d", env.Code, bizerr.CodeParamInvalid)
	}
	env = e.do(t, http.MethodGet, "/api/manage/content-guard/records/999", nil)
	if env.Code != bizerr.CodeNotFound {
		t.Fatalf("记录不存在业务码 = %d，期望 %d", env.Code, bizerr.CodeNotFound)
	}
	env = e.do(t, http.MethodGet, "/api/manage/content-guard/records/abc", nil)
	if env.Code != bizerr.CodeParamInvalid {
		t.Fatalf("非法 id 业务码 = %d，期望 %d", env.Code, bizerr.CodeParamInvalid)
	}
}

func TestSanctions_ListApplyRelease(t *testing.T) {
	e := newTestEnv(t)
	seedDeny(t, e, "毒品", 3)
	submit(t, e, "我要毒品")

	env := e.mustOK(t, http.MethodGet, "/api/manage/content-guard/sanctions?active_only=true", nil)
	var list struct {
		List  []model.GuardSanction `json:"list"`
		Total int64                 `json:"total"`
	}
	if err := json.Unmarshal(env.Data, &list); err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if list.Total != 1 || len(list.List) != 1 {
		t.Fatalf("处置数 = %d/%d，期望 1", list.Total, len(list.List))
	}
	id := list.List[0].ID

	// 人工施加：非法档位应被 4000 拦下。
	env = e.do(t, http.MethodPost, "/api/manage/content-guard/sanctions", map[string]any{
		"subject_type": model.SubjectUser, "subject_id": "u_2", "level": "forever",
	})
	if env.Code != bizerr.CodeParamInvalid {
		t.Fatalf("非法档位业务码 = %d，期望 %d", env.Code, bizerr.CodeParamInvalid)
	}
	// 缺 subject_id 同理。
	env = e.do(t, http.MethodPost, "/api/manage/content-guard/sanctions", map[string]any{
		"subject_type": model.SubjectUser, "level": model.SanctionLevelTemp,
	})
	if env.Code != bizerr.CodeParamInvalid {
		t.Fatalf("缺 subject_id 业务码 = %d，期望 %d", env.Code, bizerr.CodeParamInvalid)
	}

	env = e.mustOK(t, http.MethodPost, "/api/manage/content-guard/sanctions", map[string]any{
		"subject_type": model.SubjectMerchant, "subject_id": "t_9",
		"kind": model.SanctionKindBlacklist, "level": model.SanctionLevelPermanent,
		"reason": "人工拉黑",
	})
	var created model.GuardSanction
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if created.Kind != model.SanctionKindBlacklist || created.Source != "manual" {
		t.Fatalf("人工处置 = %+v", created)
	}

	env = e.mustOK(t, http.MethodPost, "/api/manage/content-guard/sanctions/"+itoa(id)+"/release",
		map[string]any{"reason": "误判"})
	var released model.GuardSanction
	if err := json.Unmarshal(env.Data, &released); err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if released.Active || released.ReleaseReason != "误判" {
		t.Fatalf("解除结果 = %+v", released)
	}

	env = e.do(t, http.MethodPost, "/api/manage/content-guard/sanctions/999/release", nil)
	if env.Code != bizerr.CodeNotFound {
		t.Fatalf("处置不存在业务码 = %d，期望 %d", env.Code, bizerr.CodeNotFound)
	}
}

func TestStats(t *testing.T) {
	e := newTestEnv(t)
	seedDeny(t, e, "毒品", 3)
	submit(t, e, "我要毒品")

	env := e.mustOK(t, http.MethodGet, "/api/manage/content-guard/stats", nil)
	var stats map[string]any
	if err := json.Unmarshal(env.Data, &stats); err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if _, ok := stats["pipeline"]; !ok {
		t.Fatalf("stats 缺 pipeline：%+v", stats)
	}
	if _, ok := stats["dictionary"]; !ok {
		t.Fatalf("stats 缺 dictionary：%+v", stats)
	}
}

func TestUnavailableRuntimeReturns503Semantics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// 未装配（Words/Policies/Records/Sanctions 缺一）时，所有端点必须是"底座未装配"，
	// 不能出现"路由通了但行为不明"的中间态。
	rt := service.NewRuntime(service.RuntimeDeps{})
	r := gin.New()
	NewGuardController(rt).RegisterRoutes(r.Group("/api"))

	cases := []struct{ method, path string }{
		{http.MethodGet, "/api/manage/content-guard/words"},
		{http.MethodGet, "/api/manage/content-guard/policies"},
		{http.MethodGet, "/api/manage/content-guard/records"},
		{http.MethodGet, "/api/manage/content-guard/sanctions"},
		{http.MethodGet, "/api/manage/content-guard/stats"},
		{http.MethodPost, "/api/manage/content-guard/scan"},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, bytes.NewReader([]byte("{}")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var env envelope
		if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
			t.Fatalf("%s %s 响应不可解析：%v", tc.method, tc.path, err)
		}
		if env.Code != bizerr.CodeInternal {
			t.Fatalf("%s %s 业务码 = %d，期望 %d（未装配）", tc.method, tc.path, env.Code, bizerr.CodeInternal)
		}
	}
}

func itoa(v uint64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
