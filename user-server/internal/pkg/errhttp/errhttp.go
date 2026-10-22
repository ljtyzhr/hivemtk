package errhttp

import (
	"net/http"
	"strings"

	bizerr "hivemtk-user/internal/domain/errors"
	"hivemtk-user/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
)

// HandleDBError 处理服务层返回的错误。
// 如果错误表示记录不存在（消息包含"不存在"或"not found"），
// 响应 404；否则响应 500 并附带操作描述。
// operation 参数用于构造 500 错误消息，如 "获取短链"、"创建活码"。
// 返回 true 表示已处理错误并写入响应，调用方应立即 return。
func HandleDBError(ctx *gin.Context, err error, operation string) bool {
	if err == nil {
		return false
	}
	if IsNotFoundError(err) {
		response.NotFound(ctx, "记录不存在")
		return true
	}
	response.ErrorFromDB(ctx, err, operation+"失败")
	return true
}

// HandleServiceError 处理服务层返回的业务错误。
// 与 HandleDBError 不同，非"记录不存在"的错误响应 400（业务校验错误），
// 适用于服务层会返回业务校验错误（如"系统模板不能修改"、"无权限修改"）的场景。
// 返回 true 表示已处理错误并写入响应，调用方应立即 return。
func HandleServiceError(ctx *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	if IsNotFoundError(err) {
		response.NotFound(ctx, "记录不存在")
		return true
	}
	response.Error(ctx, http.StatusBadRequest, err.Error())
	return true
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	// 上游/渠道失败**不是**"记录不存在"。
	//
	// 这一段是必需的：下面的文本判定是"整条错误链里含'不存在'或'not found'就算"，
	// 而渠道侧的原文常常自带这些词 —— 阿里云短信回
	// "Specified access key is not found."、腾讯回 "key not found" ——
	// 于是"密钥填错了"被翻译成 404「记录不存在」，把操作者引去查记录而不是改密钥。
	//
	// 判据不能是"文本里有没有 not found"（那正是错的地方），
	// 必须是错误**自己声明**它是上游失败：带 5xxx 平台/内部类业务码的 BizError
	// （见 bizerr.IsUpstreamFailure）。service 层用
	// bizerr.Wrap(bizerr.CodePlatformUnavail, ...) 标记渠道错误。
	if bizerr.IsUpstreamFailure(err) {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "不存在") || strings.Contains(msg, "not found")
}

// IsNotFoundError 公开版 isNotFoundError(供跨包调用)
func IsNotFoundError(err error) bool {
	return isNotFoundError(err)
}
