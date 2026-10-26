// Package service 实现 wafadmin 的业务逻辑层。
// 该层只依赖仓储接口与基础设施包，不感知 HTTP 细节。
package service

import (
	"errors"
	"fmt"
	"net/http"
)

// Kind 表示业务错误的类别。
type Kind string

// 业务错误类别，与 HTTP 状态码一一对应。
const (
	KindInvalid      Kind = "invalid"
	KindUnauthorized Kind = "unauthorized"
	KindForbidden    Kind = "forbidden"
	KindNotFound     Kind = "not_found"
	KindConflict     Kind = "conflict"
	KindInternal     Kind = "internal"
)

// Fault 定义一类业务错误，通过 New 生成具体错误。
type Fault struct {
	Kind    Kind
	Message string
}

// New 生成具体业务错误；format 为空时使用类别默认文案。
func (f Fault) New(format string, args ...any) *Error {
	message := f.Message
	if format != "" {
		message = fmt.Sprintf(format, args...)
	}
	return &Error{Kind: f.Kind, Message: message}
}

// Wrap 把底层错误包装为业务错误，原始错误保留在 Err 中。
func (f Fault) Wrap(err error, format string, args ...any) *Error {
	wrapped := f.New(format, args...)
	wrapped.Err = err
	return wrapped
}

// 预置的业务错误类别。
var (
	// ErrInvalid 参数校验失败。
	ErrInvalid = Fault{Kind: KindInvalid, Message: "请求参数不合法"}
	// ErrUnauthorized 未登录或凭证错误。
	ErrUnauthorized = Fault{Kind: KindUnauthorized, Message: "登录状态无效，请重新登录"}
	// ErrForbidden 无权操作。
	ErrForbidden = Fault{Kind: KindForbidden, Message: "无权执行该操作"}
	// ErrNotFound 资源不存在。
	ErrNotFound = Fault{Kind: KindNotFound, Message: "资源不存在"}
	// ErrConflict 资源冲突（重名等）。
	ErrConflict = Fault{Kind: KindConflict, Message: "资源冲突"}
	// ErrInternal 内部错误。
	ErrInternal = Fault{Kind: KindInternal, Message: "服务内部错误"}
)

// Error 是带类别的业务错误。
type Error struct {
	Kind    Kind
	Message string
	Err     error
}

// Error 实现 error 接口。
func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// Unwrap 支持 errors.Is / errors.As 穿透到底层错误。
func (e *Error) Unwrap() error { return e.Err }

// Status 返回对应的 HTTP 状态码。
func (e *Error) Status() int {
	switch e.Kind {
	case KindInvalid:
		return http.StatusBadRequest
	case KindUnauthorized:
		return http.StatusUnauthorized
	case KindForbidden:
		return http.StatusForbidden
	case KindNotFound:
		return http.StatusNotFound
	case KindConflict:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

// Code 返回对应的业务响应码。
func (e *Error) Code() int {
	switch e.Kind {
	case KindInvalid:
		return 1001
	case KindUnauthorized:
		return 1002
	case KindForbidden:
		return 1003
	case KindNotFound:
		return 1004
	case KindConflict:
		return 1005
	default:
		return 1500
	}
}

// AsError 从任意 error 中提取业务错误。
func AsError(err error) (*Error, bool) {
	target := new(*Error)
	if errors.As(err, target) && *target != nil {
		return *target, true
	}
	return nil, false
}
