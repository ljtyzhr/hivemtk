// T4 状态机与 QA 解析单元测试（表驱动，无 DB 依赖的纯逻辑部分）。
package model

import "testing"

func TestKBCrawlerSourceTransitionAllowed(t *testing.T) {
	cases := []struct {
		from, to string
		want     bool
	}{
		{"active", "paused", true},
		{"active", "error", true},
		{"paused", "active", true},
		{"error", "active", true},
		{"paused", "error", false},
		{"error", "paused", false},
		{"active", "active", false},
		{"", "active", false},
	}
	for _, tc := range cases {
		if got := KBCrawlerSourceTransitionAllowed(tc.from, tc.to); got != tc.want {
			t.Errorf("%s→%s = %v, want %v", tc.from, tc.to, got, tc.want)
		}
	}
}

func TestQACandidateTransitionAllowed(t *testing.T) {
	cases := []struct {
		from, to string
		want     bool
	}{
		{"pending_review", "approved", true},
		{"pending_review", "rejected", true},
		{"approved", "indexed", true},
		{"pending_review", "indexed", false}, // 必须先 approve
		{"rejected", "approved", false},      // 终态
		{"indexed", "pending_review", false}, // 终态
		{"approved", "rejected", false},
	}
	for _, tc := range cases {
		if got := QACandidateTransitionAllowed(tc.from, tc.to); got != tc.want {
			t.Errorf("%s→%s = %v, want %v", tc.from, tc.to, got, tc.want)
		}
	}
}
