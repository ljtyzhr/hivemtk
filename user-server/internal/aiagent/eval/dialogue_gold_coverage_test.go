package eval

// dialogue_gold_coverage_test.go P0-1 残点 2 对账闸：gold 集的意图类与否决链覆盖计数。
//
// 背景：清单要求「61 条的意图类/否决链覆盖计数（对账脚本是否在库）」——
// 此前只有下限断言（≥50 条），没有覆盖结构断言：删掉整类场景（如 objection_timing）
// 样本总数仍可能 ≥50，评测却对整类意图失明。本闸把覆盖口径钉成可执行判据。
//
// 口径：
//   - 场景类（scene）覆盖：五类 objection 链（price/need/trust/competitor/timing）
//     每类 ≥2 条，其余核心场景（quote/purchase/handoff/aftersales/complaint）各 ≥2 条；
//   - ID 唯一：重复 ID 会让 bad_case 追迹与失败定位指向歧义行；
//   - 退化相可用性：带 degraded 的样本 ≥2（否则退化相整相跳过，AC③ 无读数）。
//
// 注意静态 gold 文件不带 case_id（bad_case 是运行期送料、样本由
// BadCaseToDialogueSample 现场构造），对账只在运行期样本上可追溯——见
// service/dialogue_eval_service_test.go TestRunGoldEvaluation_BadCaseFeed。

import (
	"testing"
)

func TestDialogueGold_SceneCoverageReconciliation(t *testing.T) {
	samples := loadGoldFixture(t)

	byScene := map[string]int{}
	seenID := map[string]int{}
	degradedReady := 0
	for i, s := range samples {
		byScene[s.Scene]++
		if s.ID == "" {
			t.Errorf("样本 #%d 缺 ID", i)
			continue
		}
		if prev, dup := seenID[s.ID]; dup {
			t.Errorf("ID %q 重复（#%d 与 #%d）", s.ID, prev, i)
		}
		seenID[s.ID] = i
		if len(s.Degraded) > 0 {
			degradedReady++
		}
	}

	// 否决链：objection_* 五类各 ≥2。
	for _, sc := range []string{
		"objection_price", "objection_need", "objection_trust",
		"objection_competitor", "objection_timing",
	} {
		if byScene[sc] < 2 {
			t.Errorf("否决链场景 %q 覆盖 %d 条，应 ≥2（整类失明风险）", sc, byScene[sc])
		}
	}
	// 核心售卖/售后/交接场景各 ≥2。
	for _, sc := range []string{"quote", "purchase", "handoff", "complaint"} {
		if byScene[sc] < 2 {
			t.Errorf("核心场景 %q 覆盖 %d 条，应 ≥2", sc, byScene[sc])
		}
	}
	if degradedReady < 2 {
		t.Errorf("带 degraded 的样本 %d 条，应 ≥2（不足则退化相整相跳过，AC③ 无读数）", degradedReady)
	}
	if len(seenID) < 50 {
		t.Errorf("去重 ID 数 = %d，应 ≥50", len(seenID))
	}
}
