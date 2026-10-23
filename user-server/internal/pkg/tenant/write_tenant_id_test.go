// write_tenant_id_test.go 钉住 WriteTenantID 的三条语义。
//
// 这个助手存在的唯一理由是「异步/批量写入器在入队那一刻必须把租户抄进行里」。
// 三条语义各对应一种真实故障，且三种故障的表现都不像「漏了租户」：
//   - 有绑定却不用绑定 ⇒ 行落错租户（on 模式被 fail-closed 拒掉，看起来像"功能没生效"）；
//   - off 模式无绑定不给默认租户 ⇒ 单机部署里合规/召回日志整批写不进去；
//   - on 模式无绑定回落默认租户 ⇒ 一次漏配变成一次静默跨租户写（最坏的一种）。
package tenant

import (
	"context"
	"testing"
)

func TestWriteTenantID(t *testing.T) {
	prev := CurrentMode()
	t.Cleanup(func() { SetMode(prev) })

	cases := []struct {
		name string
		mode Mode
		ctx  context.Context
		want uint
	}{
		{
			name: "off 模式有绑定：绑定优先（离线部署里也能显式指定归属）",
			mode: ModeOff,
			ctx:  WithTenantID(context.Background(), 7),
			want: 7,
		},
		{
			name: "off 模式无绑定：默认租户（与 ContextWithDefault 同口径）",
			mode: ModeOff,
			ctx:  context.Background(),
			want: DefaultTenantID,
		},
		{
			name: "off 模式 nil ctx：默认租户，不 panic",
			mode: ModeOff,
			ctx:  nil,
			want: DefaultTenantID,
		},
		{
			name: "on 模式有绑定：绑定值",
			mode: ModeOn,
			ctx:  WithTenantID(context.Background(), 21),
			want: 21,
		},
		{
			name: "on 模式无绑定：返回 0 让调用方丢弃并出声",
			mode: ModeOn,
			ctx:  context.Background(),
			want: 0,
		},
		{
			name: "on 模式 nil ctx：返回 0",
			mode: ModeOn,
			ctx:  nil,
			want: 0,
		},
		{
			name: "on 模式 WithTenantID(0) 视同无绑定",
			mode: ModeOn,
			ctx:  WithTenantID(context.Background(), 0),
			want: 0,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			SetMode(c.mode)
			if got := WriteTenantID(c.ctx); got != c.want {
				t.Fatalf("WriteTenantID = %d，期望 %d（mode=%s）", got, c.want, c.mode)
			}
		})
	}
}

// TestWriteTenantID_NeverFallsBackToDefaultInOnMode 单独再钉一次最坏的那一条。
//
// 与上表的 "on 模式无绑定" 重复是故意的：这条不是"其中一种分支"，而是整个助手的
// 安全底线。它一旦回落默认租户，漏配就从「日志里一行 error」变成「A 租户的数据
// 静默写进租户 1」，且没有任何一处会报错。
func TestWriteTenantID_NeverFallsBackToDefaultInOnMode(t *testing.T) {
	prev := CurrentMode()
	t.Cleanup(func() { SetMode(prev) })
	SetMode(ModeOn)

	if got := WriteTenantID(context.Background()); got == DefaultTenantID {
		t.Fatalf("on 模式无绑定 ctx 返回了默认租户 %d ⇒ 一次漏配会变成一次静默跨租户写", got)
	}
	if got := WriteTenantID(context.Background()); got != 0 {
		t.Fatalf("on 模式无绑定 ctx 必须返回 0，got %d", got)
	}
}
