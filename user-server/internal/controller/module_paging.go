package controller

import (
	"github.com/gin-gonic/gin"
)

// pageOf / pageSizeOf 从 query 读分页参数并**钳制到合法区间**。
//
// 为什么不直接 strconv.Atoi 了事：page_size 透传一个 10^9 会让 SQL 一次拉
// 全表，而负数/非数字会让 offset 变成负值从而报错但看不出原因。
// 钳制在这里做一次，控制器里就不用每处都判。
func pageOf(c *gin.Context) int {
	p := queryInt(c, "page", 1)
	if p < 1 {
		return 1
	}
	return p
}

func pageSizeOf(c *gin.Context) int {
	s := queryInt(c, "page_size", 20)
	if s < 1 || s > 200 {
		return 20
	}
	return s
}

// 说明：queryInt 已在 csat.go 里存在（全 controller 包共用），此处不再重复定义。
