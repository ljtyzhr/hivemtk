package errors

// 契约：「上游/渠道失败」与「内部故障」是**两件事**，不能都算成上游失败。
//
// 坑：判据如果按码段（5xxx 整段）判，CodeInternal(5000) 也会被算成"上游失败"
// ⇒ 内部故障（写库失败之类）会被报成 502 Bad Gateway。
// 502 的语义是"网关/上游给的东西不能用"，运维照着查会去查渠道，
// 而真因在自己的库里 —— 查错方向比报 500 更费时间。
//
// 本文件把这条边界钉死：
//  1. CodePlatformUnavail ⇒ 是上游失败；
//  2. CodeInternal ⇒ **不是**上游失败（内部故障，保持 500 语义）；
//  3. 非 BizError ⇒ 不是上游失败。

import (
	goerrors "errors"
	"testing"
)

func TestIsUpstreamFailure(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"平台不可达（显式声明的上游失败）",
			Wrap(CodePlatformUnavail, "渠道不可达", goerrors.New("connection refused")), true},
		{"内部故障（CodeInternal 不是上游失败）",
			Wrap(CodeInternal, "保存资产主表失败", goerrors.New("connection refused")), false},
		{"参数错误", New(CodeParamInvalid, "参数错"), false},
		{"未找到", New(CodeNotFound, "不存在"), false},
		{"裸 error", goerrors.New("boom"), false},
		{"nil", nil, false},
		{"包过一层的上游失败",
			goerrors.New("外层: " + Wrap(CodePlatformUnavail, "渠道不可达", goerrors.New("x")).Error()), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsUpstreamFailure(tc.err); got != tc.want {
				t.Errorf("IsUpstreamFailure(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// TestIsUpstreamFailure_WrappedKeepsClassification 包一层之后仍要认得出。
func TestIsUpstreamFailure_WrappedKeepsClassification(t *testing.T) {
	inner := Wrap(CodePlatformUnavail, "渠道不可达", goerrors.New("x"))
	// 注意：用字符串包一层（fmt.Errorf("%s", inner.Error())）会**丢**类型信息，
	// errors.As 认不出来 —— 这正是本判据要靠 **errors.As / %w** 而不是文本的原因。
	// 下面断言的是"用 %w / errors.Join 包"才认得出 —— 文档说的用法就是 %w。
	if !IsUpstreamFailure(goerrors.Join(inner)) {
		t.Errorf("用 errors.Join 包装后仍应认得出上游失败")
	}
	if !IsUpstreamFailure(goerrors.Join(goerrors.New("外层"), inner)) {
		t.Errorf("多层包装后仍应认得出上游失败")
	}
}
