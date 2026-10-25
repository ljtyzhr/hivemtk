package router

import (
	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"hivemtk-user/internal/repository"
)

// setupDictionaryRoutes 注册业务字典路由。
//
// 分两道：登录档挂两条只读（终端页面要同一份值域来翻译存量行、渲染下拉框），
// admin 挂两条写（字典改一下就让管理端与历史数据的对应关系静默错位，
// 不是普通运维该碰的东西）。
//
// 读侧为什么也在登录档：approvalTask / badCase / customerSession 三个**非超管也在用**的
// 页面把 `GET /api/manage/dictionaries` 当值域来源，读侧挂硬超管守卫时它们的下拉框
// 对客服和坐席恒空（前端吞掉 403，症状是"页面打开了但选项目是空的"）。
// 写侧留硬守卫：新增/改词条是全局治理动作，与"这页能不能渲染"无关。
func setupDictionaryRoutes(plainAuth, admin *gin.RouterGroup, db *gorm.DB) {
	svc := service.NewDictionaryService(repository.NewDictionaryRepository(db))
	ctrl := controller.NewDictionaryController(svc)

	// 只读下发挂 plainAuth：终端页面渲染存量行时需要同一份值域，
	// 但不该因此给普通用户开管理端的写路径。两条返回体一致，差别只在鉴权。
	plainAuth.GET("/config/dictionaries/:parent", ctrl.PublicDictionary)

	plainAuth.GET("/manage/dictionaries", ctrl.ListParents)
	plainAuth.GET("/manage/dictionaries/:parent", ctrl.GetDictionary)

	admin.PUT("/manage/dictionaries/:parent/items", ctrl.UpsertItem)
	admin.PUT("/manage/dictionaries/:parent/transitions", ctrl.ReplaceTransitions)
}
