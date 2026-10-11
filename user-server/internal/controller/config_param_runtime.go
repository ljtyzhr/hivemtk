package controller

import (
	"net/http"
	"sort"
	"strings"

	"hivemtk-user/internal/pkg/utils/logger"

	"github.com/gin-gonic/gin"
)

// runtimePublicGroups 是 `GET /api/config/runtime` 允许读取的组白名单。
//
// 为什么必须白名单而不是「全量吐回去」：
//  1. `/api/manage/config-params` 是**管理端全量列表**，每条带 Name/Description/Min/Max/
//     Restart/ReadOnly。把它暴露给终端用户等于把整套运维决策面摊开——里面既有
//     `misc.upload_max_size_mb` 这种可直接放开的口子，也有 `read_only` 标记为系统
//     推导的键。运维改一格是一回事，让任意登录用户看见全部键名与取值是另一回事。
//  2. 前端真正需要的只是「这一格当前填了几」，即 value 一个字段。给它一个扁平的
//     `group.key -> value` 映射就够，不给任何可解释性字段。
//
// 新增组要在这里显式登记，并写清楚为什么前端需要它。
var runtimePublicGroups = map[string]struct{}{
	// 前端自己实现的 WS 客户端重连/心跳/ACK 批次等旋钮
	//（utils/agentSocket.js、utils/chatSocket.js、utils/journeyTracker.js）。
	"frontend_ws": {},
	// 前端视图里的阈值类常量（轮询间隔、扫描页大小、失败上限等）。
	"frontend_ui": {},
	// 列表分页的默认页大小与单页上限：前端 api/*.js 里各写一份的
	// limit||20 / pageSize||20 应统一读这里。
	"pagination": {},
}

// Runtime GET /api/config/runtime?groups=frontend_ws,pagination
//
// 只读运行时配置，供前端一次性拉取后缓存。响应只有 value：
//
//	{"code":0,"data":{"frontend_ws.max_reconnect_attempts_default":"10", ...}}
//
// 判据（三条都写死在代码里，不给「可配置」的口子）：
//  1. 只返回白名单内的组；传入的 groups 里不在白名单的**静默忽略**——不报错也不回显，
//     免得调用方拿它当「探测有哪些组」的手段。
//  2. 只返回 value（字符串原样）。Name/Description/Min/Max/Restart 一个都不给。
//  3. 不读数据库时（参数中心不可用）返回空 map + code 0，前端据此用代码兜底值——
//     不能因为运维配置面挂了就让整个 SPA 白屏。
func (c *ConfigParamController) Runtime(ctx *gin.Context) {
	requested := splitGroups(ctx.Query("groups"))
	if len(requested) == 0 {
		// 不传 groups 时返回白名单全部，省得前端每次 hardcode 一份清单
		// （那份清单会和这里漂移，然后前端悄悄少读一组）。
		for g := range runtimePublicGroups {
			requested = append(requested, g)
		}
		sort.Strings(requested)
	}
	allowed := make([]string, 0, len(requested))
	for _, g := range requested {
		if _, ok := runtimePublicGroups[g]; ok {
			allowed = append(allowed, g)
		}
	}

	// c.svc 为 nil（装配顺序出错、或测试直接 NewConfigParamController(nil)）时
	// 返回空配置：Runtime 是前端启动链上的一环，它 panic 会让整个 SPA 打不开。
	out := make(map[string]string, 32)
	if c.svc == nil {
		ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": out, "message": "ok"})
		return
	}
	if len(allowed) > 0 {
		for _, g := range allowed {
			list, err := c.svc.ListByGroup(ctx.Request.Context(), g)
			if err != nil {
				// 单组读失败不应让整次请求失败：其余组照常返回，失败的组由前端用
				// 兜底值顶着（这正是「配置面挂了不白屏」的判据 3）。
				logger.Warnf("[ConfigParam] Runtime group=%s err=%v", g, err)
				continue
			}
			for i := range list {
				out[g+"."+list[i].Key] = list[i].Value
			}
		}
	}
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": out, "message": "ok"})
}

// splitGroups 解析 groups 查询参数：逗号分隔，去空白，去重，保序去空。
func splitGroups(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	seen := make(map[string]struct{}, 4)
	out := make([]string, 0, 4)
	for _, part := range strings.Split(raw, ",") {
		g := strings.TrimSpace(part)
		if g == "" {
			continue
		}
		if _, dup := seen[g]; dup {
			continue
		}
		seen[g] = struct{}{}
		out = append(out, g)
	}
	return out
}

// AnyRuntimeGroupPublic 供测试断言白名单集合本身（不暴露内部 map）。
func AnyRuntimeGroupPublic(groups ...string) bool {
	for _, g := range groups {
		if _, ok := runtimePublicGroups[g]; !ok {
			return false
		}
	}
	return true
}
