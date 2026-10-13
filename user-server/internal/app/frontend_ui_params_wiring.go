package app

import (
	"context"

	contentctrl "hivemtk-user/internal/content/controller"
	appsvc "hivemtk-user/internal/service"
)

// WireFrontendUIConfigParams 把 frontend_ui 组里**后端也要认**的那几条接到读取口。
//
// frontend_ui 组历史上只有前端读（runtime 下发给浏览器做本地拦截与文案），后端一处不摸，
// 于是「前端放行、后端拦死」成了这一组默认形态：运维把上限调大后，请求到了服务端仍按写死的
// 旧值拒。本函数是这个默认形态的第一个例外。
//
// material_max_size_mb 为什么走注入而不是像 misc.upload_max_size_mb 那样在 controller 里直连
// service.GlobalConfigParam()：internal/content 整个子模块不 import internal/service
// （全树 grep 零命中），依赖链在下层，反向摸上去会打断这条边界；
// 所以沿用 content 侧既有范式（internal/content/service 的 maxFlowDelaySecondsProvider 同款），
// 由装配层注入闭包。
//
// Get* 调用与键名字面量必须写在同一行：scripts/check-config-param-readpoints.py
// 认的就是这个形状。
func WireFrontendUIConfigParams() []string {
	bg := context.Background()
	cp := appsvc.GlobalConfigParam()

	contentctrl.SetMaterialMaxSizeMBProvider(func() int {
		return cp.GetInt(bg, "frontend_ui", "material_max_size_mb", contentctrl.DefaultMaterialMaxSizeMB)
	})

	return []string{"frontend_ui.material_max_size_mb"}
}
