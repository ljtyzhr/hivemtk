// chat_channel_allowed_origins_test.go 渠道「允许的 origin」入参形状的回归。
//
// 背景（任务三逐路由 curl 实测）：GET /api/chat-channels/:id 返回的
// allowed_origins 是**字符串**（model.ChatChannel.AllowedOrigins 是 text 列），
// 而写接口的入参曾被声明成 []string。于是「照着读到的原样回写」这一条极常见的
// 客户端动作会直接被 gin 反序列化拒掉：
//
//	json: cannot unmarshal string into Go struct field
//	CreateChannelRequest.allowed_origins of type []string
//
// 同时前端（Create.vue / Edit.vue / InstallGuide.vue）发的是**数组**。
// 所以入参必须同时吃下两种形状，否则总有一类客户端 100% 失败。
//
// 本用例钉住：
//   - 字符串形态、数组形态都能解析出同一组 origin
//   - 归一化后落库形态是逗号分隔串（与列类型一致）
//   - 空白项被丢弃、分隔符（, ; 空白 换行）都被认
//   - 非法形态（数字/对象）被拒，而不是静默当成空
package service

import (
	"encoding/json"
	"testing"
)

func TestAllowedOriginsInput_UnmarshalAcceptsStringAndArray(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{
			name: "字符串形态（与 GET 回读的形状一致）",
			raw:  `"https://a.com,https://b.com"`,
			want: []string{"https://a.com", "https://b.com"},
		},
		{
			name: "数组形态（前端 Create.vue 的形状）",
			raw:  `["https://a.com","https://b.com"]`,
			want: []string{"https://a.com", "https://b.com"},
		},
		{
			name: "单元素字符串",
			raw:  `"*"`,
			want: []string{"*"},
		},
		{
			name: "混合分隔符（逗号/分号/空白/换行）都认",
			raw:  `"https://a.com; https://b.com\nhttps://c.com"`,
			want: []string{"https://a.com", "https://b.com", "https://c.com"},
		},
		{
			name: "空白项被丢弃",
			raw:  `"https://a.com,,  ,https://b.com"`,
			want: []string{"https://a.com", "https://b.com"},
		},
		{
			name: "空串 = 没有 origin",
			raw:  `""`,
			want: []string{},
		},
		{
			name: "null = 没有 origin",
			raw:  `null`,
			want: []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got AllowedOriginsInput
			if err := json.Unmarshal([]byte(tc.raw), &got); err != nil {
				t.Fatalf("解析 %s 失败: %v", tc.raw, err)
			}
			values := got.Values()
			if len(values) != len(tc.want) {
				t.Fatalf("origin 数量 = %d（%v），期望 %d（%v）", len(values), values, len(tc.want), tc.want)
			}
			for i := range values {
				if values[i] != tc.want[i] {
					t.Errorf("origin[%d] = %q，期望 %q", i, values[i], tc.want[i])
				}
			}
		})
	}
}

func TestAllowedOriginsInput_RejectsNonStringShapes(t *testing.T) {
	for _, raw := range []string{`123`, `{"a":1}`, `true`} {
		var got AllowedOriginsInput
		if err := json.Unmarshal([]byte(raw), &got); err == nil {
			t.Errorf("%s 应当被拒，实际解析成功：%v", raw, got.Values())
		}
	}
}

func TestAllowedOriginsInput_NormalizesToCommaSeparated(t *testing.T) {
	// 落库形态必须是逗号分隔串（allowed_origins 是 text 列）。
	var got AllowedOriginsInput
	if err := json.Unmarshal([]byte(`["https://a.com","https://b.com"]`), &got); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if got.String() != "https://a.com,https://b.com" {
		t.Errorf("归一化结果 = %q，期望 %q", got.String(), "https://a.com,https://b.com")
	}
	// 读接口口径是字符串 ⇒ MarshalJSON 也要吐字符串，否则 echo 回去会自伤。
	blob, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	if string(blob) != `"https://a.com,https://b.com"` {
		t.Errorf("序列化结果 = %s，期望字符串形态", blob)
	}
}

func TestAllowedOriginsInput_EmptyIsDetected(t *testing.T) {
	var empty AllowedOriginsInput
	if err := json.Unmarshal([]byte(`""`), &empty); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if !empty.IsEmpty() {
		t.Error("空串应被判定为没有配置 origin")
	}
	var one AllowedOriginsInput
	if err := json.Unmarshal([]byte(`"https://a.com"`), &one); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if one.IsEmpty() {
		t.Error("有 origin 时不应判定为空")
	}
}
