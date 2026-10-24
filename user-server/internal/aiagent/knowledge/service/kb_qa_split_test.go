package service

import (
	"strings"
	"testing"
)

// parseQAOutput 四类用例（T4 设计 §9.1）：合法 JSON / 带多余文字 / 空数组 / 截断。
func TestParseQAOutput(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantN   int
		wantErr bool
	}{
		{
			name:  "合法JSON数组",
			in:    `[{"question":"如何重置密码?","answer":"进入设置页重置","answer_anchor":"重置密码"}]`,
			wantN: 1,
		},
		{
			name:  "带前后多余文字",
			in:    "好的，以下是提取结果：\n[{\"question\":\"Q1\",\"answer\":\"A1\"},{\"question\":\"Q2\",\"answer\":\"A2\"}]\n以上共2条。",
			wantN: 2,
		},
		{
			name:  "空数组（纯导航页）",
			in:    "[]",
			wantN: 0,
		},
		{
			name:    "截断（缺右括号）",
			in:      `[{"question":"Q1","answer":"A1"`,
			wantErr: true,
		},
		{
			name:    "纯文字无数组",
			in:      "抱歉，该页面无可提取内容。",
			wantErr: true,
		},
		{
			name:    "数组里是对象以外的元素",
			in:      `["abc"]`,
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items, err := parseQAOutput(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("期望报错，得到 %d 条", len(items))
				}
				return
			}
			if err != nil {
				t.Fatalf("意外报错: %v", err)
			}
			if len(items) != tc.wantN {
				t.Fatalf("条数不符: got %d want %d", len(items), tc.wantN)
			}
			for _, it := range items {
				if strings.TrimSpace(it.Question) == "" || strings.TrimSpace(it.Answer) == "" {
					t.Fatalf("question/answer 为空: %+v", it)
				}
			}
		})
	}
}

func TestEstimateTokens(t *testing.T) {
	if got := estimateTokens(""); got != 0 {
		t.Fatalf("空串应为 0, got %d", got)
	}
	if got := estimateTokens("abcdefgh"); got != 5 { // 8*2/3
		t.Fatalf("英文估算不符: got %d want 5", got)
	}
	// 中文 3 个 rune（注："测试串" 是 3 字符）
	if got := estimateTokens("测试串"); got != 2 { // 3*2/3
		t.Fatalf("中文估算不符: got %d want 2", got)
	}
}
