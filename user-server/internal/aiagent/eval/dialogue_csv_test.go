package eval

// dialogue_csv_test.go P0-1 残点 5：CSV 编解码的可逆性与拒绝面。
//
// 字节对账判据：export(样本) → import → export 逐字节相等（规范形）；
// 拒绝面：错表头/错 JSON/空 id/重复 id/无 user 回合 —— 任一都必须整体拒绝，
// 不做部分导入（分母对不上的评测集比没有更危险）。

import (
	"encoding/csv"
	"strings"
	"testing"
)

func sampleSetForCSV() []DialogueSample {
	return []DialogueSample{
		{
			ID: "s-1", Scene: "price_inquiry", CaseID: "bc-1", Label: "kb_missing", FixLayer: "knowledge",
			Turns: []DialogueTurn{
				{Role: DialogueTurnRoleUser, Text: "多少钱？"},
				{
					Role: DialogueTurnRoleAssistant, Text: "4999 元，含 12 个月。",
					ExpectedAction: "quote", ExpectedIntent: "ask_price",
					MustContain: []string{"4999", "12 个月"},
				},
			},
			Degraded: []DialogueTurn{
				{Role: DialogueTurnRoleUser, Text: "多少钱？"},
				{Role: DialogueTurnRoleAssistant, Text: "请稍等。", ExpectedAction: "defer"},
			},
		},
		{
			ID: "s-2", Scene: "objection_price",
			Turns: []DialogueTurn{
				{Role: DialogueTurnRoleUser, Text: "太贵了。"},
				{Role: DialogueTurnRoleAssistant, Text: "可以按年付优惠。"},
			},
		},
	}
}

func TestDialogueCSV_RoundTripByteIdentical(t *testing.T) {
	var first strings.Builder
	if err := ExportDialogueCSV(&first, sampleSetForCSV()); err != nil {
		t.Fatalf("首次导出失败: %v", err)
	}
	samples, err := ImportDialogueCSV(strings.NewReader(first.String()))
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("导入条数 = %d want 2", len(samples))
	}
	if samples[0].CaseID != "bc-1" || samples[0].FixLayer != "knowledge" || samples[0].Label != "kb_missing" {
		t.Errorf("bad_case 追迹列丢失: %+v", samples[0])
	}
	if len(samples[0].Degraded) != 2 || samples[0].Degraded[1].ExpectedAction != "defer" {
		t.Errorf("退化 transcript 丢失: %+v", samples[0].Degraded)
	}
	if got := samples[0].Turns[1].MustContain; len(got) != 2 || got[0] != "4999" {
		t.Errorf("must_contain 丢失: %+v", got)
	}

	var second strings.Builder
	if err := ExportDialogueCSV(&second, samples); err != nil {
		t.Fatalf("二次导出失败: %v", err)
	}
	if first.String() != second.String() {
		t.Errorf("字节对账失败：\n第一次:\n%s\n第二次:\n%s", first.String(), second.String())
	}
}

func TestDialogueCSV_Rejects(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "错表头",
			in:   "id,scene,label,turns\nx,y,z,[]\n",
			want: "表头",
		},
		{
			name: "turns 非 JSON",
			in:   "id,scene,label,fix_layer,case_id,turns,degraded\ns-1,quote,,, ,not-json,\n",
			want: "turns",
		},
		{
			name: "空 id",
			in:   "id,scene,label,fix_layer,case_id,turns,degraded\n,quote,,,,[]" + ",\n",
			want: "id 不能为空",
		},
		{
			name: "重复 id",
			in: csvText(t, [][]string{
				{"s-1", "quote", "", "", "", `[{"role":"user","text":"q"}]`, ""},
				{"s-1", "quote", "", "", "", `[{"role":"user","text":"q"}]`, ""},
			}),
			want: "重复",
		},
		{
			name: "无 user 回合",
			in: csvText(t, [][]string{
				{"s-1", "quote", "", "", "", `[{"role":"assistant","text":"a"}]`, ""},
			}),
			want: "user",
		},
		{
			name: "无数据行",
			in:   strings.Join(DialogueCSVHeader, ",") + "\n",
			want: "无数据行",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ImportDialogueCSV(strings.NewReader(tc.in))
			if err == nil {
				t.Fatalf("应拒绝，实际通过")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("错误 = %q，应含 %q", err.Error(), tc.want)
			}
		})
	}
}

func TestDialogueCSV_TemplateImportable(t *testing.T) {
	samples, err := ImportDialogueCSV(strings.NewReader(string(DialogueCSVTemplate())))
	if err != nil {
		t.Fatalf("模板自身不可导入: %v", err)
	}
	if len(samples) != 1 || samples[0].ID != "example-001" {
		t.Errorf("模板样本 = %+v", samples)
	}
}

// csvText 用 csv.Writer 构造夹具（手写拼接会漏引号转义，测出来的错误是解析层噪声）。
func csvText(t *testing.T, rows [][]string) string {
	t.Helper()
	var b strings.Builder
	w := csv.NewWriter(&b)
	if err := w.Write(DialogueCSVHeader); err != nil {
		t.Fatalf("写表头失败: %v", err)
	}
	for _, r := range rows {
		if err := w.Write(r); err != nil {
			t.Fatalf("写行失败: %v", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		t.Fatalf("csv 刷盘失败: %v", err)
	}
	return b.String()
}
