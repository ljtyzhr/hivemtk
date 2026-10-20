package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"hivemtk-user/internal/browser_automation/model"
	"hivemtk-user/internal/browser_automation/repository"

	"gorm.io/datatypes"
)

// R27-1 I5 审计导出：SessionExport 归并会话+步+命令流+LLM 成本账+裁剪摘要的契约锁。
// 归属校验/仓储缺位置空不报错（与 D1 读侧同纪律）。
// 导出包里为什么要有摘要分量：命令流被按界裁掉之后，导出包里没有它就只能显示一个空数组，
// 「这段没发生过」与「这段被治理裁掉了」便同形——那正是 A6 立项要消灭的形状。

type exportSessionRepo struct {
	fakeSessionRepo
	sess *model.BrowserSession
}

func (f *exportSessionRepo) GetByID(_ context.Context, id, userID uint) (*model.BrowserSession, error) {
	if f.sess == nil || f.sess.ID != id || f.sess.UserID != userID {
		return nil, errors.New("not found")
	}
	return f.sess, nil
}

type exportStepRepo struct {
	repository.BrowserStepRepository
	steps []*model.BrowserStep
}

func (e *exportStepRepo) ListBySessionID(_ context.Context, _ uint) ([]*model.BrowserStep, error) {
	return e.steps, nil
}

type exportCmdLogRepo struct {
	repository.BrowserCommandLogRepository
	logs []*model.BrowserCommandLog
}

func (e *exportCmdLogRepo) ListBySessionID(_ context.Context, _ uint) ([]*model.BrowserCommandLog, error) {
	return e.logs, nil
}

type exportPlanRepo struct {
	repository.BrowserLLMPlanRepository
	plans []*model.BrowserLLMPlan
}

func (e *exportPlanRepo) ListBySessionID(_ context.Context, _ uint, limit int) ([]*model.BrowserLLMPlan, error) {
	if limit > 0 && limit < len(e.plans) {
		return e.plans[:limit], nil
	}
	return e.plans, nil
}

// exportDigestRepo 裁剪摘要读侧（/ A6）。导出包里带它，才有办法把
// 「command_log 是空的」与「command_log 被按界裁掉了」这两种解释分开说。
type exportDigestRepo struct {
	repository.BrowserAuditDigestRepository
	digests []*model.BrowserAuditDigest
}

func (e *exportDigestRepo) ListBySessionID(_ context.Context, _ uint) ([]*model.BrowserAuditDigest, error) {
	return e.digests, nil
}

// exportErrRepo 四个分量共用的「装配了但这一枪打空」桩：只把 ListBySessionID 换成报错，
// 其余方法走嵌入的接口（永不触达，nil 接口也没关系）。
type exportErrRepo struct {
	repository.BrowserStepRepository
	err error
}

func (e *exportErrRepo) ListBySessionID(_ context.Context, _ uint) ([]*model.BrowserStep, error) {
	return nil, e.err
}

// TestSessionExportFailsWhenAComponentQueryFails 「分量没装配 ⇒ 置空不报错」与
// 「分量装配了却查失败 ⇒ 报错」是两件事，现在被写成同一件。
//
// 为什么这是缺陷：导出包是给审计员看的，空数组在包里只有一种解释——「这一段没发生过」。
// 步查询报错却交出 steps=nil，读的人便认定「这个会话没跑过任何步」，而真相是库在这一刻
// 没答上话。这与本函数文档里 A6 立项要消灭的形状完全同形（「空数组没人分得清是
// 「没发生」还是「被裁了」」），也与监控盘把 8 处错误吞成全零同形。
// 归属预检查（GetByID）挡在前面，所以这条路不会因为租户 ctx 未绑定而误红：
// 能走到分量查询的 ctx 必然已经过归属校验。
func TestSessionExportFailsWhenAComponentQueryFails(t *testing.T) {
	sess := &model.BrowserSession{ID: 9, UserID: 3, Status: "completed"}
	sr := &exportSessionRepo{sess: sess}
	boom := errors.New("export: 分量查询失败")

	for _, tc := range []struct {
		name string
		run  func(*SessionService)
	}{
		{"步流水", func(s *SessionService) { s.stepRepo = &exportErrRepo{err: boom} }},
		{"命令流", func(s *SessionService) {
			s.SetCommandLogRepository(&exportCmdLogErrRepo{err: boom})
		}},
		{"LLM 成本账", func(s *SessionService) {
			s.SetLLMPlanRepository(&exportPlanErrRepo{err: boom})
		}},
		{"裁剪摘要", func(s *SessionService) {
			s.SetAuditDigestRepository(&exportDigestErrRepo{err: boom})
		}},
	} {
		s := NewSessionService(sr, &exportStepRepo{steps: []*model.BrowserStep{{ID: 1, SessionID: 9}}}, nil)
		tc.run(s)
		if _, _, _, _, _, err := s.SessionExport(context.Background(), 9, 3); !errors.Is(err, boom) {
			t.Errorf("%s 分量查询失败时导出仍返回成功（err=%v）：交出的是一份看着完整的空分量审计包", tc.name, err)
		}
	}
}

type exportCmdLogErrRepo struct {
	repository.BrowserCommandLogRepository
	err error
}

func (e *exportCmdLogErrRepo) ListBySessionID(_ context.Context, _ uint) ([]*model.BrowserCommandLog, error) {
	return nil, e.err
}

type exportPlanErrRepo struct {
	repository.BrowserLLMPlanRepository
	err error
}

func (e *exportPlanErrRepo) ListBySessionID(_ context.Context, _ uint, _ int) ([]*model.BrowserLLMPlan, error) {
	return nil, e.err
}

type exportDigestErrRepo struct {
	repository.BrowserAuditDigestRepository
	err error
}

func (e *exportDigestErrRepo) ListBySessionID(_ context.Context, _ uint) ([]*model.BrowserAuditDigest, error) {
	return nil, e.err
}

func TestSessionExportAggregates(t *testing.T) {
	sess := &model.BrowserSession{ID: 9, UserID: 3, Status: "completed"}
	sr := &exportSessionRepo{sess: sess}
	steps := &exportStepRepo{steps: []*model.BrowserStep{{ID: 1, SessionID: 9, Action: "click", Status: "success"}}}
	cl := &exportCmdLogRepo{logs: []*model.BrowserCommandLog{{ID: 1, SessionID: 9, Seq: 1, Direction: "command", Action: "click", Ok: nil}}}
	pl := &exportPlanRepo{plans: []*model.BrowserLLMPlan{
		{ID: 1, SessionID: 9, Kind: "plan", Goal: "g", TokenIn: 10, TokenOut: 2, Snapshot: "BIG-IGNORED"},
		{ID: 2, SessionID: 9, Kind: "judge", Goal: "g", Reasoning: "approve"},
	}}
	dg := &exportDigestRepo{digests: []*model.BrowserAuditDigest{
		{ID: 7, SessionID: 9, RowCount: 3, FirstSeq: 1, LastSeq: 3, Ordinal: 1,
			BatchDigest: "aa", ChainHash: "bb"},
	}}
	s := NewSessionService(sr, steps, nil)
	s.SetCommandLogRepository(cl)
	s.SetLLMPlanRepository(pl)
	s.SetAuditDigestRepository(dg)

	got, gs, gl, gp, gd, err := s.SessionExport(context.Background(), 9, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != 9 || len(gs) != 1 || len(gl) != 1 || len(gp) != 2 || len(gd) != 1 {
		t.Fatalf("归并不全: sess=%v steps=%d logs=%d plans=%d digests=%d", got, len(gs), len(gl), len(gp), len(gd))
	}
	if gp[0].Kind != "plan" || gp[0].TokenIn != 10 {
		t.Errorf("LLM 成本账字段丢失: %+v", gp[0])
	}
	// Snapshot 大文本不进导出行（SessionExportRow 无该字段即证明）
	blob, _ := json.Marshal(gp)
	if strings.Contains(string(blob), "BIG-IGNORED") {
		t.Error("导出不得携带 snapshot 大文本")
	}
	// 摘要行整体进包（含批/链哈希与 seq 区间）：审计员拿的就是这一份，字段丢了就自证不了
	dblob, _ := json.Marshal(gd)
	if !strings.Contains(string(dblob), `"batch_digest":"aa"`) ||
		!strings.Contains(string(dblob), `"row_count":3`) {
		t.Errorf("摘要行没完整进导出包: %s", dblob)
	}

	// 归属越界 → 报错（GetByID 校验 userID）
	if _, _, _, _, _, err := s.SessionExport(context.Background(), 9, 999); err == nil {
		t.Error("越权归属必须报错")
	}

	// 仓储缺位（nil）不报错、置空（D1 同纪律）
	s2 := NewSessionService(sr, steps, nil)
	if _, _, logs, plans, digests, err := s2.SessionExport(context.Background(), 9, 3); err != nil ||
		logs != nil || plans != nil || digests != nil {
		t.Errorf("未装配分量应置空不报错: %v %v %v %v", err, logs, plans, digests)
	}

	// llm_plan steps JSON 原样透传为字节
	s3 := NewSessionService(sr, &exportStepRepo{}, nil)
	s3.SetLLMPlanRepository(&exportPlanRepo{plans: []*model.BrowserLLMPlan{
		{ID: 1, SessionID: 9, Kind: "plan", Steps: datatypes.JSON(`[{"action":"click"}]`)},
	}})
	if _, _, _, rows, _, _ := s3.SessionExport(context.Background(), 9, 3); len(rows) != 1 || string(rows[0].Steps) != `[{"action":"click"}]` {
		t.Errorf("steps JSON 透传失败: %+v", rows)
	}
}
