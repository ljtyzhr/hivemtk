package middleware

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils/logger"
)

// 公开资源（分享页、短链、活码）没有登录态，租户必须由资源自身携带的线索解析：
// 先用系统作用域按唯一键反查 tenant_id，再把该租户绑定进请求 ctx，后续所有
// 业务读取/统计写入都落在资源所属租户内。反查失败不绑定——on 模式下下游
// 隔离 callback 会 fail-closed（表现为资源不存在），绝不回落到默认租户。

type publicLookup struct {
	table   string
	column  string
	param   string
	numeric bool
}

func publicResourceTenant(db *gorm.DB, l publicLookup) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !tenant.IsMultiTenant() {
			c.Next()
			return
		}
		raw := c.Param(l.param)
		if raw == "" || db == nil {
			c.Next()
			return
		}
		var value any = raw
		if l.numeric {
			id, err := strconv.ParseUint(raw, 10, 32)
			if err != nil {
				c.Next()
				return
			}
			value = uint(id)
		}
		// 业务键复合化后同键可能在多个租户各有一行（T008），按键 First() 会把访客
		// 随机指派给某个租户。只接受"全部命中行同属一个租户"的自证结果。
		tenantID, err := tenant.SingleTenantByKey(c.Request.Context(), db, l.table, l.column+" = ?", value)
		if err != nil {
			logger.Warnf("[tenant] 公开入口租户解析拒绝 table=%s %s=%v: %v", l.table, l.column, raw, err)
			c.Next()
			return
		}
		if tenantID == 0 {
			c.Next()
			return
		}
		c.Request = c.Request.WithContext(tenant.OnlyTenant(c.Request.Context(), tenantID))
		c.Next()
	}
}

// PublicIDTenant 按数值主键 :id 解析公开资源所属租户
func PublicIDTenant(db *gorm.DB, table string) gin.HandlerFunc {
	return publicResourceTenant(db, publicLookup{table: table, column: "id", param: "id", numeric: true})
}

// PublicStringIDTenant 按字符串主键 :id 解析公开资源所属租户
func PublicStringIDTenant(db *gorm.DB, table string) gin.HandlerFunc {
	return publicResourceTenant(db, publicLookup{table: table, column: "id", param: "id"})
}

// PublicCodeTenant 按 :code 命中指定列解析公开资源所属租户
func PublicCodeTenant(db *gorm.DB, table, column string) gin.HandlerFunc {
	return publicResourceTenant(db, publicLookup{table: table, column: column, param: "code"})
}

// tenantHintFromRequest 取调用方显式声明的租户短码：查询参数优先，其次请求头。
// 公开链路里"按键自证"之外的那一条声明通道，访客渠道与 MCP 单值凭证共用同一形状。
func tenantHintFromRequest(c *gin.Context) string {
	code := strings.TrimSpace(c.Query("tenant_code"))
	if code == "" {
		code = strings.TrimSpace(c.GetHeader("X-Tenant-Code"))
	}
	return code
}

// PublicTenantHint 把调用方显式带来的租户短码绑进请求 ctx（仅 on 模式）。
//
// 什么时候需要它：渠道业务键（`default`、`{platform}_card`）在多租户下**注定**重复
// ——每租户接入时自动建一份，按键自证只能得到"歧义"。此时唯一不失真的判据是调用方
// 自己声明"我这颗浮标属于哪个租户"；带上它，后续读取走绑定作用域而不是系统作用域，
// 渠道行、会话与消息都留在声明的那个租户里。
//
// 只填"尚未绑定"的空位，绝不覆写已有归属（渠道行自证、登录态中间件都排在它前面）。
// 短码解析不出时既不绑定也不拦请求：这道中间件的职责是"给得出线索时给"，
// 该不该拒由下游各自的判据决定（访客链最终会因渠道歧义被 401）。
func PublicTenantHint(resolveTenant TenantCodeResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !tenant.IsMultiTenant() || resolveTenant == nil {
			c.Next()
			return
		}
		if id, bound := tenant.FromContext(c.Request.Context()); bound && id != 0 {
			c.Next()
			return
		}
		code := tenantHintFromRequest(c)
		if code == "" {
			c.Next()
			return
		}
		id, err := resolveTenant(c.Request.Context(), code)
		if err != nil {
			logger.Warnf("[tenant] 公开入口租户短码解析失败 code=%q path=%s: %v", code, c.FullPath(), err)
			c.Next()
			return
		}
		if id == 0 {
			c.Next()
			return
		}
		c.Request = c.Request.WithContext(tenant.OnlyTenant(c.Request.Context(), id))
		c.Next()
	}
}
