package eval

// dialogue_csv.go P0-1 残点 5：对话评测集 CSV 导入/导出（对标 Dify 批量评测形态）。
//
// 为什么 CSV 而不是只用 JSON：运营同学改评测集走表格工具（Excel/Sheets），
// CSV 是他们的原生界面；turns/degraded 两个多回合列用规范 JSON 承载——
// 一行一个样本，列数固定，表格里可读可改，也不引入第二套 schema。
//
// 字节对账口径：导出是**规范形**（固定列序、JSON 无多余空白、\r\n 之外不带
// 额外行尾），因此 export(import(export(x))) == export(x) 逐字节成立——
// 这正是 AC「导出与导入字节对账」的判据。

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// DialogueCSVHeader CSV 列序（导出与导入共用，改序即破坏字节对账，勿动）。
var DialogueCSVHeader = []string{"id", "scene", "label", "fix_layer", "case_id", "turns", "degraded"}

// DialogueCSVTemplate 模板行：给运营同学的填写样例（一行一条）。
func DialogueCSVTemplate() []byte {
	samples := []DialogueSample{{
		ID:    "example-001",
		Scene: "price_inquiry",
		Turns: []DialogueTurn{
			{Role: DialogueTurnRoleUser, Text: "你们年付多少钱？"},
			{
				Role:           DialogueTurnRoleAssistant,
				Text:           "年付 4999 元，含 12 个月全功能。",
				ExpectedAction: "quote",
				ExpectedIntent: "ask_price",
				MustContain:    []string{"4999", "12 个月"},
			},
		},
	}}
	var buf strings.Builder
	_ = ExportDialogueCSV(&buf, samples)
	return []byte(buf.String())
}

// ExportDialogueCSV 把样本按规范形写 CSV。
func ExportDialogueCSV(w io.Writer, samples []DialogueSample) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(DialogueCSVHeader); err != nil {
		return fmt.Errorf("dialogue csv: 写表头失败: %w", err)
	}
	for i, s := range samples {
		turns, err := json.Marshal(s.Turns)
		if err != nil {
			return fmt.Errorf("dialogue csv: 第 %d 条 turns 序列化失败: %w", i+1, err)
		}
		var degraded []byte
		if len(s.Degraded) > 0 {
			if degraded, err = json.Marshal(s.Degraded); err != nil {
				return fmt.Errorf("dialogue csv: 第 %d 条 degraded 序列化失败: %w", i+1, err)
			}
		}
		row := []string{s.ID, s.Scene, s.Label, s.FixLayer, s.CaseID, string(turns), string(degraded)}
		if err := cw.Write(row); err != nil {
			return fmt.Errorf("dialogue csv: 写第 %d 条失败: %w", i+1, err)
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return fmt.Errorf("dialogue csv: 刷盘失败: %w", err)
	}
	return nil
}

// ImportDialogueCSV 解析 CSV 为样本。任何结构错误都整体拒绝（不做部分导入）——
// 半套评测集比没有更危险：跑起来的分母会对不上运营看到的条数。
func ImportDialogueCSV(r io.Reader) ([]DialogueSample, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = len(DialogueCSVHeader)
	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("dialogue csv: 读表头失败: %w", err)
	}
	for i, want := range DialogueCSVHeader {
		if strings.TrimSpace(header[i]) != want {
			return nil, fmt.Errorf("dialogue csv: 表头第 %d 列 = %q，应为 %q（请用模板）", i+1, header[i], want)
		}
	}

	var samples []DialogueSample
	seen := map[string]int{}
	for line := 2; ; line++ {
		row, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("dialogue csv: 第 %d 行读取失败: %w", line, err)
		}
		s, perr := parseDialogueCSVRow(row)
		if perr != nil {
			return nil, fmt.Errorf("dialogue csv: 第 %d 行: %w", line, perr)
		}
		if prev, dup := seen[s.ID]; dup {
			return nil, fmt.Errorf("dialogue csv: 第 %d 行 id %q 与第 %d 行重复", line, s.ID, prev)
		}
		seen[s.ID] = line
		samples = append(samples, s)
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("dialogue csv: 无数据行")
	}
	return samples, nil
}

func parseDialogueCSVRow(row []string) (DialogueSample, error) {
	s := DialogueSample{
		ID:       strings.TrimSpace(row[0]),
		Scene:    strings.TrimSpace(row[1]),
		Label:    strings.TrimSpace(row[2]),
		FixLayer: strings.TrimSpace(row[3]),
		CaseID:   strings.TrimSpace(row[4]),
	}
	if s.ID == "" {
		return s, fmt.Errorf("id 不能为空")
	}
	if s.Scene == "" {
		return s, fmt.Errorf("scene 不能为空（用于覆盖对账）")
	}
	if err := json.Unmarshal([]byte(row[5]), &s.Turns); err != nil {
		return s, fmt.Errorf("turns 不是合法 JSON 回合数组: %w", err)
	}
	if len(s.Turns) == 0 {
		return s, fmt.Errorf("turns 至少要有 1 个回合")
	}
	hasUser := false
	for i, t := range s.Turns {
		if t.Role != DialogueTurnRoleUser && t.Role != DialogueTurnRoleAssistant {
			return s, fmt.Errorf("turns[%d].role = %q，只支持 user/assistant", i, t.Role)
		}
		if t.Role == DialogueTurnRoleUser {
			hasUser = true
		}
	}
	if !hasUser {
		return s, fmt.Errorf("turns 缺 user 回合（题面）")
	}
	if v := strings.TrimSpace(row[6]); v != "" {
		if err := json.Unmarshal([]byte(v), &s.Degraded); err != nil {
			return s, fmt.Errorf("degraded 不是合法 JSON 回合数组: %w", err)
		}
	}
	return s, nil
}
