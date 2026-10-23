// contract_test.go 租户端契约版本号与"平台要求升级"状态的判据（SaaS 审计 A4）。
package contract

import (
	"testing"
	"time"
)

// TestContractVersionPinned 钉住契约版本字面量。
//
// 与平台端 `internal/contract/contract_test.go` 的同名用例是**一对**：任一侧单方面改
// Version，另一侧的所有请求都会开始 426。两侧各钉一份，改的人必须两边一起改。
func TestContractVersionPinned(t *testing.T) {
	if Version != 1 {
		t.Fatalf("契约版本已变（当前 %d，用例钉的是 1）：这是跨仓协议号，改它必须与平台端 "+
			"internal/contract.Version 同批升级", Version)
	}
	if HeaderName != "X-Contract-Version" {
		t.Fatalf("契约版本请求头名变了（%q）：平台端按同名常量读取，两侧必须逐字一致", HeaderName)
	}
	if HeaderValue() != "1" {
		t.Fatalf("HeaderValue() 应是十进制字面量 \"1\"（平台端 strconv.Atoi 解析），实际 %q", HeaderValue())
	}
}

// TestRecordAndClearUpgradeRequired 状态机：记录 → 可见 → 清除 → 不可见。
func TestRecordAndClearUpgradeRequired(t *testing.T) {
	ClearUpgradeRequired()
	t.Cleanup(ClearUpgradeRequired)

	if _, ok := UpgradeRequired(); ok {
		t.Fatal("清空后不该有状态 —— 否则管理端会显示一条不存在的告警")
	}

	RecordUpgradeRequired("平台最低支持 v2")
	st, ok := UpgradeRequired()
	if !ok {
		t.Fatal("记录后必须可见（这条状态的唯一用途就是给管理员看）")
	}
	if st.Message != "平台最低支持 v2" {
		t.Errorf("原因应原样保留，实际 %q", st.Message)
	}
	if st.Declared != Version {
		t.Errorf("应带上本机声明的版本 %d，实际 %d", Version, st.Declared)
	}
	if st.Since == "" {
		t.Error("应记录首次/最近被拒时间（运维据此判断「是不是一直在打」）")
	}
	if _, err := time.Parse(time.RFC3339, st.Since); err != nil {
		t.Errorf("Since 应是 RFC3339，实际 %q（%v）", st.Since, err)
	}

	ClearUpgradeRequired()
	if _, ok := UpgradeRequired(); ok {
		t.Fatal("清除后必须不可见（正对照：没有它，一个「只记不清」的实现也能让上面全绿）")
	}
}

// TestRecordIsIdempotentAndCounts 重复上报：文案不堆叠，次数累加。
//
// 为什么次数要单独留一格：平台会对**每一个**请求回 426，同一条原因会被上报成百上千次。
// 若把文案拼接起来，管理端会显示一屏幕相同文字，真正的信息（"一直在被拒"）反而看不出来；
// 而完全不计数则分不清"刚才偶发一次"和"已经连续被拒一小时"。
func TestRecordIsIdempotentAndCounts(t *testing.T) {
	ClearUpgradeRequired()
	t.Cleanup(ClearUpgradeRequired)

	RecordUpgradeRequired("平台最低支持 v2")
	first, _ := UpgradeRequired()

	RecordUpgradeRequired("平台最低支持 v2")
	RecordUpgradeRequired("平台最低支持 v2")
	third, ok := UpgradeRequired()
	if !ok {
		t.Fatal("重复上报后仍应可见")
	}
	if third.Message != first.Message {
		t.Errorf("同一条原因重复上报后文案变了：%q → %q（会被拼成几百行相同告警）",
			first.Message, third.Message)
	}
	if third.Count != 3 {
		t.Errorf("累计次数应为 3，实际 %d（这一格用来区分「偶发一次」与「一直在被拒」）", third.Count)
	}
}

// TestRecordOverwritesWithLatestReason 原因变了就换成新的。
//
// 平台在两个方向上给的文案不同（升租户 / 升平台）。运维要做的动作完全不一样，
// 所以后一条必须覆盖前一条，而不是"第一次那条钉死"。
func TestRecordOverwritesWithLatestReason(t *testing.T) {
	ClearUpgradeRequired()
	t.Cleanup(ClearUpgradeRequired)

	RecordUpgradeRequired("平台最低支持 v2：请升级 user-server")
	RecordUpgradeRequired("本机声明 v3 高于平台 v2：请升级平台")

	st, _ := UpgradeRequired()
	if st.Message != "本机声明 v3 高于平台 v2：请升级平台" {
		t.Errorf("原因应被最新一条覆盖，实际 %q", st.Message)
	}
}
