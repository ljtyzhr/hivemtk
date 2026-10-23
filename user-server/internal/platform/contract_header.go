// contract_header.go 是**出站请求的唯一构造入口**，以及"平台对契约版本的态度"的唯一记账口
// （SaaS 审计 A4）。
//
// # 为什么必须收成一处
//
// 平台侧在 `/merchant-api`、`/llm-api/v1`、`/api/platform` 三个组的门口各挂了一道
// `ContractHandshake()`：不声明 `X-Contract-Version` 的请求一律 **HTTP 426**。
// 租户端的出站请求散在 6 个文件、7 处 `http.NewRequest` 里。如果逐个手加请求头，
// 那么"新写一个客户端忘了带"的后果是**那条功能恒 426**，而它在日志里长得像
// "平台把我们的某个接口关掉了"——排查方向会直接歪掉。所以：凡是发请求，都从这里造。
//
// 反过来，记账也只在这里判断：`noteContractVerdict` 是**唯一**决定"这次响应算不算
// 平台拒绝了我们的契约版本"的地方。它只对 `contractGuardedPrefixes` 里的路径生效 ——
// 对没挂闸的端点（`/health`、登录、贡献者接口）清状态会造成"平台明明在拒我们，
// 但一次健康检查把告警清了"的假绿。
package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"hivemtk-user/internal/pkg/contract"
)

// contractGuardedPrefixes 平台端挂了 `ContractHandshake()` 的前缀（**租户侧的对账单**）。
//
// 与平台端 `scripts/check-merchant-scope.py` 的 `CONTRACT_GUARDED` 一一对应。
// 它在这里的用途只有一个：判断"这次响应有没有资格被当作契约版本的裁决"。
// 少登记一个 ⇒ 那条通道的 426 不会被记录，管理端看不到告警；多登记一个 ⇒
// 一次普通 200 会把别处的 426 清掉。两种都静默，所以清单要跟着平台侧一起改。
var contractGuardedPrefixes = []string{
	"/merchant-api/",
	"/llm-api/v1/",
	"/api/platform/",
}

// isContractGuardedPath 判断某个出站路径是否受平台契约闸约束。
func isContractGuardedPath(path string) bool {
	for _, p := range contractGuardedPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// newPlatformRequest 构造**每一次**出站请求。ctx 传 nil 时用 Background。
//
// 契约版本头在这里无条件打上，包括那些平台**没有**挂闸的端点（`/health`、`/api/auth/login`、
// 贡献者接口）。这不是"顺手多加一个头"，而是故意不留一个开关：一旦这里有个
// `if guarded { Set(...) }`，那么"哪些前缀挂了闸"就又多了一份要在两处同步的清单，
// 而多出来的那份迟早会漂 —— 多带一个没人读的头，代价是零。
func newPlatformRequest(ctx context.Context, method, url string, body []byte) (*http.Request, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set(contract.HeaderName, contract.HeaderValue())
	return req, nil
}

// noteContractVerdict 是"平台是否要求本机升级"的**唯一判定点**。
//
// 三档，各对应一种真实的失效：
//
//	426  ⇒ 记下原因（管理端可见）。平台侧的 `UpgradeRequired` 把码同时写进 HTTP 状态
//	       与信封，所以这里优先取信封 msg（它对运维说清了"升哪一端"），取不到再退回原文。
//	200  ⇒ 清掉告警。**只有受闸路径的 200 才有这个资格**：闸放行了，说明契约是匹配的。
//	其它 ⇒ 不动。500/401/超时的原因与契约无关，拿它清告警等于把真问题抹掉。
//
// body 允许为 nil（有些调用点不读响应体）：此时 426 的原因退回平台状态码文案。
func noteContractVerdict(path string, statusCode int, body []byte) {
	if !isContractGuardedPath(path) {
		return
	}
	switch statusCode {
	case http.StatusUpgradeRequired:
		contract.RecordUpgradeRequired(contractRefusalMessage(body))
	case http.StatusOK:
		contract.ClearUpgradeRequired()
	}
}

// contractRefusalMessage 从 426 响应体里取一句给人看的原因。
func contractRefusalMessage(body []byte) string {
	var env struct {
		Msg string `json:"msg"`
	}
	if len(body) > 0 && json.Unmarshal(body, &env) == nil {
		if msg := strings.TrimSpace(env.Msg); msg != "" {
			return msg
		}
	}
	return "平台返回 426 Upgrade Required：本机声明的契约版本与平台不兼容"
}
