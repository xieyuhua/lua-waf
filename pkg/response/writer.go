package response

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// 业务响应码。
const (
	CodeOK           = 0
	CodeInvalid      = 1001
	CodeUnauthorized = 1002
	CodeForbidden    = 1003
	CodeNotFound     = 1004
	CodeConflict     = 1005
	CodeInternal     = 1500
)

// maxBodySize 限制请求体大小，避免内存放大攻击。
const maxBodySize = 4 << 20

// Envelope 是接口的统一响应结构。
type Envelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Writer 负责统一的 JSON 出参与入参绑定。
type Writer struct{}

// OK 返回成功响应。
func (Writer) OK(w http.ResponseWriter, data any) {
	writeEnvelope(w, http.StatusOK, Envelope{Code: CodeOK, Message: "ok", Data: data})
}

// Fail 返回失败响应。
func (Writer) Fail(w http.ResponseWriter, status, code int, message string) {
	writeEnvelope(w, status, Envelope{Code: code, Message: message})
}

// Bind 解析 JSON 请求体。
func (Writer) Bind(r *http.Request, payload any) error {
	if r.Body == nil {
		return errors.New("请求体为空")
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxBodySize))
	if err := decoder.Decode(payload); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("请求体为空")
		}
		return err
	}
	return nil
}

// writeEnvelope 输出响应并设置通用响应头。
func writeEnvelope(w http.ResponseWriter, status int, envelope Envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope)
}
