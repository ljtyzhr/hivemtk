package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/service"
)

// 管理端「写配置」这条链路的端到端用例：请求体 → handler 绑定 → service 校验 →
// 落库 → 缓存失效 → 审计。
//
// 为什么要有这一份：读侧（Get* 走参数表）先前有整套门与用例，写侧却只被 service 层
// 用例覆盖，而那些用例是直接调 svc.UpdateValue(ctx, ..., "60", ...) 传**字符串**进去的。
// 浏览器发的却是 {"value":60}（数字）与 {"value":true}（布尔），HTTP 绑定层收不收这份
// 载荷从来没有被测过——绑定失败会退回「参数错误」，页面显示保存失败，而所有 Go 侧用例
// 全绿。这一份把「前端真正发出的形状」钉成判据。
//
// 不调 service.SeedConfigParams：它会 SetGlobal 并把三个包级 setter 注到本测试库上，
// 同包后续用例读全局时打到的是已经 Drop 的库（跨用例串状态）。这里只需要
// 「参数表里有生产形状的行」，按 DefaultParamDefs 整份落库即可。

func newConfigParamWriteFixture(t *testing.T) (*gin.Engine, *gorm.DB, *service.ConfigParamService) {
	t.Helper()
	database := testutil.NewTestDBOrSkip(t,
		&model.ConfigParam{}, &model.ConfigParamGroup{}, &model.ConfigParamAuditLog{})

	defs := service.DefaultParamDefs()
	rows := make([]model.ConfigParam, 0, len(defs))
	for _, def := range defs {
		rows = append(rows, model.ConfigParam{
			Group:            def.Group,
			Key:              def.Key,
			Name:             def.Name,
			Description:      def.Description,
			ValueType:        def.ValueType,
			Value:            def.DefaultValue,
			DefaultValue:     def.DefaultValue,
			Min:              def.Min,
			Max:              def.Max,
			Step:             def.Step,
			Unit:             def.Unit,
			Risk:             def.Risk,
			Deprecated:       def.Deprecated,
			DeprecatedReason: def.DeprecatedReason,
			ReadOnly:         def.ReadOnly,
			Restart:          def.Restart,
			Category:         def.Category,
		})
	}
	if err := database.CreateInBatches(rows, 100).Error; err != nil {
		t.Fatalf("灌参数行失败：%v", err)
	}

	svc := service.NewConfigParamService(database)
	ctrl := NewConfigParamController(svc)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/manage/config-params", ctrl.List)
	r.GET("/api/manage/config-params/audit-logs", ctrl.AuditLogs)
	r.PUT("/api/manage/config-params/:group/:key", ctrl.Update)
	r.POST("/api/manage/config-params/:group/:key/reset", ctrl.ResetToDefault)
	r.POST("/api/manage/config-params/:group/reset", ctrl.BulkResetGroup)
	return r, database, svc
}

// envelope 管理端统一信封（HTTP 状态恒 200，判定在 code 里）。
type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func doEnvelope(t *testing.T, r *gin.Engine, method, path, body string) envelope {
	t.Helper()
	req, err := http.NewRequest(method, path, bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("%s %s 状态码 = %d（管理端约定恒 200），体 = %s", method, path, w.Code, w.Body.String())
	}
	var e envelope
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatalf("%s %s 响应不是 JSON：%v 体=%s", method, path, err, w.Body.String())
	}
	return e
}

func doJSON(t *testing.T, r *gin.Engine, method, path, body string) (int, string) {
	t.Helper()
	e := doEnvelope(t, r, method, path, body)
	return e.Code, e.Message
}

func storedValue(t *testing.T, db *gorm.DB, group, key string) string {
	t.Helper()
	var p model.ConfigParam
	if err := db.Where(&model.ConfigParam{Group: group, Key: key}).First(&p).Error; err != nil {
		t.Fatalf("读回 %s.%s 失败：%v", group, key, err)
	}
	return p.Value
}

func putValue(t *testing.T, r *gin.Engine, group, key, rawJSONValue string) (int, string) {
	t.Helper()
	return doJSON(t, r, http.MethodPut,
		fmt.Sprintf("/api/manage/config-params/%s/%s", group, key),
		`{"value":`+rawJSONValue+`}`)
}

// TestUpdateAcceptsEveryScalarJSONShape 前端各类型编辑器真正发出去的载荷形状。
// 判据是两端一起断：接口收下（code 0）**且**库里落的是规范字符串。
func TestUpdateAcceptsEveryScalarJSONShape(t *testing.T) {
	r, db, svc := newConfigParamWriteFixture(t)
	_ = svc
	cases := []struct {
		name  string
		group string
		key   string
		raw   string // 请求体里 value 的 JSON 字面量
		want  string // 期望落库的字符串
	}{
		{"int 数字载荷（前端 el-input-number 的形状）", "bridge", "ingest_max_body_bytes", "8388608", "8388608"},
		{"int 字符串载荷", "bridge", "ingest_max_messages", "500", "500"},
		{"float 数字载荷", "agent_llm", "temperature", "0.7", "0.7"},
		{"duration 数字载荷（裸数字按秒）", "bridge", "sse_heartbeat_interval", "30", "30"},
		{"duration 带单位字符串载荷", "bridge", "sse_heartbeat_interval", `"30s"`, "30s"},
		{"bool 布尔载荷（前端 el-switch 的形状）", "cache", "faq_answer_enabled", "false", "false"},
		{"bool 字符串载荷", "cache", "faq_answer_enabled", `"true"`, "true"},
		{"string 字符串载荷", "lead", "industry_profile", `"制造业咨询"`, "制造业咨询"},
		{"大整数不许变成指数记法", "bridge", "ingest_max_body_bytes", "67108864", "67108864"},
	}
	for _, tc := range cases {
		code, msg := putValue(t, r, tc.group, tc.key, tc.raw)
		if code != 0 {
			t.Errorf("%s：PUT %s.%s = %s 被拒（code=%d msg=%q）", tc.name, tc.group, tc.key, tc.raw, code, msg)
			continue
		}
		if got := storedValue(t, db, tc.group, tc.key); got != tc.want {
			t.Errorf("%s：库里 = %q，期望 %q", tc.name, got, tc.want)
		}
	}
}

// TestUpdateRejectsNonScalarAndUnparsablePayloads 拒绝面同样要断言，否则
// 「什么都收下」和「只收标量」在这份用例里长得一样。
func TestUpdateRejectsNonScalarAndUnparsablePayloads(t *testing.T) {
	r, db, _ := newConfigParamWriteFixture(t)
	const group, key = "bridge", "ingest_max_body_bytes"
	before := storedValue(t, db, group, key)

	cases := []struct {
		name string
		body string
	}{
		{"缺 value 字段", `{}`},
		{"value 为 null", `{"value":null}`},
		{"value 为对象", `{"value":{"a":1}}`},
		{"value 为数组", `{"value":[1,2]}`},
		{"value 为 int 参数写非数字", `{"value":"abc"}`},
		{"int 越下界", `{"value":1}`},
		{"int 越上界", `{"value":70000000}`},
		{"空 body", ``},
	}
	for _, tc := range cases {
		e := doEnvelope(t, r, http.MethodPut, "/api/manage/config-params/"+group+"/"+key, tc.body)
		if e.Code == 0 {
			t.Errorf("%s：本该拒掉，却回 code 0（体=%v）", tc.name, e)
		}
		if got := storedValue(t, db, group, key); got != before {
			t.Errorf("%s：被拒之后库里值却变了：%q → %q", tc.name, before, got)
		}
	}
}

// TestUpdateRejectsBoolParamGarbage bool 型必须按 bool 校验。
// 读取侧 GetBool 用的是 strconv.ParseBool，解析失败静默回落兜底值——
// 写侧若放行 "banana"，页面显示「保存成功」、开关却一动不动，
// 与 duration 分支当年「解析失败直接 return nil」是同一个病。
func TestUpdateRejectsBoolParamGarbage(t *testing.T) {
	r, db, _ := newConfigParamWriteFixture(t)
	const group, key = "cache", "faq_answer_enabled"
	if _, msg := putValue(t, r, group, key, `"banana"`); msg == "" {
		t.Fatal("bool 参数写入非布尔值却没报错")
	}
	if code, _ := putValue(t, r, group, key, `"banana"`); code == 0 {
		t.Errorf("bool 参数写入 \"banana\" 被收下，库里现值 %q", storedValue(t, db, group, key))
	}
	// 正控制：同一条参数写真布尔必须成功，证明上一句拦的不是「这条参数整体不可写」。
	if code, msg := putValue(t, r, group, key, "true"); code != 0 {
		t.Fatalf("正控制失败：bool=true 应可写，实际 code=%d msg=%q", code, msg)
	}
	if got := storedValue(t, db, group, key); got != "true" {
		t.Errorf("正控制：库里 = %q，期望 true", got)
	}
}

// TestUpdateReadOnlyParamSaysReadOnly 只读参数的拒绝理由要说「只读」。
// 曾经返回 gorm.ErrRecordNotFound，页面读起来像「这条参数不存在」，
// 而它明明在列表里看得见——把一个明确的失败说成一个需要人推理的矛盾。
func TestUpdateReadOnlyParamSaysReadOnly(t *testing.T) {
	r, _, _ := newConfigParamWriteFixture(t)
	code, msg := putValue(t, r, "knowledge", "embedding_dimension", "2048")
	if code == 0 {
		t.Fatal("只读参数被写成功了")
	}
	if !strings.Contains(msg, "只读") {
		t.Errorf("只读参数的拒绝理由应说明「只读」，实际 msg=%q", msg)
	}
	if strings.Contains(strings.ToLower(msg), "record not found") {
		t.Errorf("只读参数报成了 record not found（参数明明在列表里）：%q", msg)
	}
}

// TestUpdateResetsCacheAndReaderSeesNewValue 写完必须让同进程的读侧立刻看到新值。
// 缓存 TTL 60s，若不失效，管理员改完上限、下一次请求仍按旧值放行，
// 而审计里已经写着改成功了。
func TestUpdateResetsCacheAndReaderSeesNewValue(t *testing.T) {
	r, _, svc := newConfigParamWriteFixture(t)
	ctx := t.Context()
	const group, key = "bridge", "ingest_max_messages"
	if got := svc.GetInt(ctx, group, key, 1); got != 200 {
		t.Fatalf("前置：初值应为种子默认 200，实际 %d", got)
	}
	if code, msg := putValue(t, r, group, key, "500"); code != 0 {
		t.Fatalf("写入失败 code=%d msg=%q", code, msg)
	}
	if got := svc.GetInt(ctx, group, key, 1); got != 500 {
		t.Errorf("写入后同进程读侧 = %d，期望 500（缓存没随写入失效）", got)
	}
}

// TestResetWritesDefaultAndAudits 单条重置：落回默认值 + 留一条 reset 审计。
func TestResetWritesDefaultAndAudits(t *testing.T) {
	r, db, _ := newConfigParamWriteFixture(t)
	const group, key = "bridge", "ingest_max_messages"
	if code, msg := putValue(t, r, group, key, "500"); code != 0 {
		t.Fatalf("前置写入失败 code=%d msg=%q", code, msg)
	}
	code, msg := doJSON(t, r, http.MethodPost, "/api/manage/config-params/"+group+"/"+key+"/reset", "")
	if code != 0 {
		t.Fatalf("重置失败 code=%d msg=%q", code, msg)
	}
	if got := storedValue(t, db, group, key); got != "200" {
		t.Errorf("重置后库里 = %q，期望回到默认 200", got)
	}
	var n int64
	if err := db.Model(&model.ConfigParamAuditLog{}).
		Where("param_group = ? AND param_key = ? AND action = ?", group, key, "reset").
		Count(&n).Error; err != nil {
		t.Fatalf("查审计失败：%v", err)
	}
	if n != 1 {
		t.Errorf("reset 审计行数 = %d，期望 1", n)
	}
}

// TestResetRejectsReadOnly 单条重置也要守只读：整组重置本来就跳过只读行，
// 单条却不判，等于「批量安全、逐条能绕过」。
func TestResetRejectsReadOnly(t *testing.T) {
	r, db, _ := newConfigParamWriteFixture(t)
	const group, key = "knowledge", "embedding_dimension"
	code, msg := doJSON(t, r, http.MethodPost, "/api/manage/config-params/"+group+"/"+key+"/reset", "")
	if code == 0 {
		t.Errorf("只读参数被单条重置接受了（库里 = %q）", storedValue(t, db, group, key))
	}
	if !strings.Contains(msg, "只读") {
		t.Errorf("只读参数重置的拒绝理由应说明「只读」，实际 %q", msg)
	}
}

// TestBulkResetGroupOnlyTouchesWritableRows 整组重置：改过的行回默认，
// 只读与下架行保持原样并且不产生审计。
func TestBulkResetGroup(t *testing.T) {
	r, db, _ := newConfigParamWriteFixture(t)
	const group = "bridge"
	// 先把两条改成各自 min（合法、但偏离默认），重置才有的可回。
	// 顺带确认越界值进不去——否则下面「回到默认」可能是「压根没写进去」的假绿。
	for key, v := range map[string]string{"ingest_max_messages": "10", "sse_heartbeat_interval": "5"} {
		if code, msg := putValue(t, r, group, key, v); code != 0 {
			t.Fatalf("前置写入 %s=%s 失败 code=%d msg=%q", key, v, code, msg)
		}
		if got := storedValue(t, db, group, key); got != v {
			t.Fatalf("前置写入未落库 %s：%q", key, got)
		}
	}
	if code, _ := putValue(t, r, group, "ingest_max_messages", "1"); code == 0 {
		t.Error("越界值 1（min=10）写进了 ingest_max_messages，下面的重置判据失去意义")
	}
	code, msg := doJSON(t, r, http.MethodPost, "/api/manage/config-params/"+group+"/reset", "")
	if code != 0 {
		t.Fatalf("整组重置失败 code=%d msg=%q", code, msg)
	}
	for key, want := range map[string]string{"ingest_max_messages": "200", "sse_heartbeat_interval": "15"} {
		if got := storedValue(t, db, group, key); got != want {
			t.Errorf("整组重置后 %s = %q，期望默认 %q", key, got, want)
		}
	}
	var n int64
	if err := db.Model(&model.ConfigParamAuditLog{}).
		Where("param_group = ? AND action = ?", group, "bulk_reset").Count(&n).Error; err != nil {
		t.Fatalf("查审计失败：%v", err)
	}
	if n != 2 {
		t.Errorf("bulk_reset 审计行数 = %d，期望 2（只给真正回到默认值的行留痕）", n)
	}
}

// TestAuditLogKeepsPlainValuesOnReadAuditLogs 读侧把加密列解回明文，
// 否则审计页读到一串密文，「谁在什么时候把哪个旋钮改成了什么」这条链就断了。
func TestAuditLogKeepsPlainValuesOnRead(t *testing.T) {
	r, _, _ := newConfigParamWriteFixture(t)
	const group, key = "bridge", "ingest_max_messages"
	if code, msg := putValue(t, r, group, key, "500"); code != 0 {
		t.Fatalf("写入失败 code=%d msg=%q", code, msg)
	}
	e := doEnvelope(t, r, http.MethodGet, "/api/manage/config-params/audit-logs?limit=10", "")
	if e.Code != 0 {
		t.Fatalf("AuditLogs code=%d msg=%q", e.Code, e.Message)
	}
	var logs []model.ConfigParamAuditLog
	if err := json.Unmarshal(e.Data, &logs); err != nil {
		t.Fatalf("审计列表解析失败：%v 体=%s", err, string(e.Data))
	}
	if len(logs) == 0 {
		t.Fatal("审计列表为空，写入没有留痕")
	}
	found := false
	for _, l := range logs {
		if l.ParamGroup != group || l.ParamKey != key {
			continue
		}
		found = true
		if l.OldValue != "200" || l.NewValue != "500" {
			t.Errorf("审计值 = %q → %q，期望 200 → 500（解密或落列有问题）", l.OldValue, l.NewValue)
		}
	}
	if !found {
		t.Errorf("审计列表里没有 %s.%s 这条（共 %d 行）", group, key, len(logs))
	}
}

// TestEveryWritableParamAcceptsItsOwnDefault 写入面的全覆盖门：
// 登记表里每一条**非只读、非下架**的参数，都必须能通过管理端接口把它自己的默认值写回去。
//
// 这一格同时钉住三件事，任何一件掉了都会红：
//  1. 载荷归一化覆盖了所有 value_type（新增一个类型却忘了在绑定层处理 → 红）；
//  2. validateValue 覆盖了所有 value_type（新增类型没有校验分支、而默认值不是该类型的合法值 → 红）；
//  3. 种子行的 ReadOnly/Deprecated 标记与「能不能写」一致（把可写的标成只读、或反过来 → 红）。
//
// 用 json.Number 原样发（int/float/duration 的数字默认值），正是前端编辑器的形状。
func TestEveryWritableParamAcceptsItsOwnDefault(t *testing.T) {
	r, db, _ := newConfigParamWriteFixture(t)
	var tried, skipped int
	for _, def := range service.DefaultParamDefs() {
		if def.ReadOnly || def.Deprecated {
			skipped++
			continue
		}
		// 数字型与布尔型按「裸 JSON 字面量」发（那是前端 el-input-number / el-switch 的真实
		// 形状），其余按字符串发。判据都是「规范化后应等于种子里的那个串」。
		var raw string
		switch def.ValueType {
		case "int", "float", "duration":
			if _, err := strconv.ParseFloat(def.DefaultValue, 64); err == nil {
				raw = def.DefaultValue
			}
		case "bool":
			if b, err := strconv.ParseBool(def.DefaultValue); err == nil &&
				strconv.FormatBool(b) == def.DefaultValue {
				raw = def.DefaultValue
			}
		}
		if raw == "" {
			b, err := json.Marshal(def.DefaultValue)
			if err != nil {
				t.Fatalf("marshal %s.%s: %v", def.Group, def.Key, err)
			}
			raw = string(b)
		}
		code, msg := putValue(t, r, def.Group, def.Key, raw)
		if code != 0 {
			t.Errorf("%s.%s（type=%s）写不回自身默认值 %s：code=%d msg=%q",
				def.Group, def.Key, def.ValueType, raw, code, msg)
			continue
		}
		if got := storedValue(t, db, def.Group, def.Key); got != def.DefaultValue {
			t.Errorf("%s.%s 落库值 = %q，期望 %q（载荷 %s）", def.Group, def.Key, got, def.DefaultValue, raw)
			continue
		}
		tried++
	}
	if tried == 0 {
		t.Fatal("一条可写参数都没测到（夹具或种子表坏了）")
	}
	t.Logf("写入面覆盖：可写 %d 条全部通过，跳过只读/下架 %d 条", tried, skipped)
}
