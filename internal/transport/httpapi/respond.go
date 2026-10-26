package httpapi

import (
	"net/http"

	"wafadmin/internal/service"
	"wafadmin/pkg/response"
)

// Responder 把业务层结果转换为统一的 HTTP 响应。
type Responder struct {
	Writer response.Writer
}

// Data 返回成功响应。
func (r Responder) Data(w http.ResponseWriter, data any) {
	r.Writer.OK(w, data)
}

// Message 返回带提示文案的成功响应。
func (r Responder) Message(w http.ResponseWriter, message string) {
	r.Writer.OK(w, map[string]string{"message": message})
}

// Error 把业务错误映射为响应码与提示。
func (r Responder) Error(w http.ResponseWriter, err error) {
	if svcErr, ok := service.AsError(err); ok {
		r.Writer.Fail(w, svcErr.Status(), svcErr.Code(), svcErr.Message)
		return
	}
	r.Writer.Fail(w, http.StatusInternalServerError, response.CodeInternal, err.Error())
}

// BindError 返回请求体解析失败的响应。
func (r Responder) BindError(w http.ResponseWriter) {
	r.Writer.Fail(w, http.StatusBadRequest, response.CodeInvalid, "请求参数格式不正确")
}
