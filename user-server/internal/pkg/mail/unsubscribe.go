// unsubscribe.go 退订出口的两种表现形态（头部机器可读 + 正文人可读），两条发信路径共用。
//
// 放在 mail 包的原因：本项目有两条独立的 SMTP 出口（单封投递与营销群发），
// 判据写进任何一条路径的包里都会被另一条各自复制一份然后漂移；
// 而漂移在合规头上不表现为报错，表现为某些邮件没有退订出口且没人知道。
package mail

import (
	"html"
	"strings"

	"gopkg.in/gomail.v2"
)

// Option 在信真正写出去之前对消息做一次追加设置。
// 做成可变参数而不是加 string 形参：现有调用方中只有部分需要退订出口。
type Option func(*gomail.Message)

const (
	listUnsubscribeHeader     = "List-Unsubscribe"
	listUnsubscribePostHeader = "List-Unsubscribe-Post"
	oneClickPostValue         = "List-Unsubscribe=One-Click"
)

// linkUsable 判"这条链接能不能进头部"。
//
// 退订链接的 base 取自 SERVER_BASE_URL —— 一个不由代码产生的值。两道风险各拦一次：
//   - CR/LF：头部值里出现换行就能劈出发信行（SMTP 注入面）；
//   - 非可打印 ASCII 与空格：gomail 会把这种头值编成 RFC 2047 encoded-word，
//     而 List-Unsubscribe 是结构化头，收件端不认 encoded-word —— 那种头存在等于不存在，
//     且 List-Unsubscribe-Post 会被读成"承诺了一键退订"，静默失效比缺失更难查。
//
// 正文页脚用更宽的一条腿（linkReadable）：它不走头编码，中文域名链接照样可点。
func linkUsable(link string) bool {
	if !linkReadable(link) {
		return false
	}
	for _, r := range link {
		// 0x21..0x7E = 可打印 ASCII 且不含空格：空格会让 stdlib 把整个值编成 encoded-word，
		// 尖括号包裹的 URI 形式也就散了。
		if r < 0x21 || r > 0x7E {
			return false
		}
	}
	return true
}

// linkReadable 正文侧判据：非空、且不含能劈开标记或伪造属性的换行。
func linkReadable(link string) bool {
	if link == "" {
		return false
	}
	return !strings.ContainsAny(link, "\r\n")
}

// Unsubscribe 一次性退订头（RFC 8058）。
//
// 两个头必须成对：只发 List-Unsubscribe，Gmail/Yahoo 对批量发件人仍按"未提供一键退订"计；
// 只发 List-Unsubscribe-Post，则是承诺了一个链接里没实现的能力。
func Unsubscribe(link string) Option {
	return func(m *gomail.Message) {
		if !linkUsable(link) {
			return
		}
		uri := link
		if !strings.HasPrefix(uri, "<") {
			uri = "<" + uri + ">"
		}
		m.SetHeader(listUnsubscribeHeader, uri)
		m.SetHeader(listUnsubscribePostHeader, oneClickPostValue)
	}
}

// AppendUnsubscribeFooter 在正文尾部补一段人可读的退订入口。
// 只有头部链接的话，纯文本视图收件人看不到退订字样，而"找不到退订入口"
// 正是 spam 举报的主要触发条件。
func AppendUnsubscribeFooter(body, link string) string {
	if !linkReadable(link) {
		return body
	}
	return body + "\n<div style=\"font-size:12px;color:#888888;margin-top:16px\">" +
		"<a href=\"" + html.EscapeString(link) + "\">退订此类邮件</a>" +
		"</div>"
}
