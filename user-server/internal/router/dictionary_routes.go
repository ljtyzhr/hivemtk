package router

import (
	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupDictionaryRoutes 注册业务字典路由。
//
// 分两道：plainAuth 挂一条只读下发（终端页面要同一份值域来翻译存量行），
// admin 挂四条 CRUD（字典改一下就让管理端与历史数据的对应关系静默错位，
// 不是普通运维该碰的东西）。
func setupDictionaryRoutes(plainAuth, admin *gin.RouterGroup, db *gorm.DB) {
	svc := service.NewDictionaryService(db)
	ctrl := controller.NewDictionaryController(svc)

	// 只读下发挂 plainAuth：终端页面渲染存量行时需要同一份值域，
	// 但不该因此给普通用户开管理端的写路径。两条返回体一致，差别只在鉴权。
	plainAuth.GET("/config/dictionaries/:parent", ctrl.PublicDictionary)

	admin.GET("/manage/dictionaries", ctrl.ListParents)
	admin.GET("/manage/dictionaries/:parent", ctrl.GetDictionary)
	admin.PUT("/manage/dictionaries/:parent/items", ctrl.UpsertItem)
	admin.PUT("/manage/dictionaries/:parent/transitions", ctrl.ReplaceTransitions)
}
