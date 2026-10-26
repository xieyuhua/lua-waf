package httpapi

import (
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"wafadmin/pkg/response"
)

// Recoverer 捕获 handler 中的 panic，返回统一错误响应。
type Recoverer struct {
	Writer response.Writer
}

// Wrap 包装下一个处理器。
func (m Recoverer) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("[error] panic %s %s: %v\n%s", r.Method, r.URL.Path, recovered, debug.Stack())
				m.Writer.Fail(w, http.StatusInternalServerError, response.CodeInternal, "服务内部错误")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// AccessLogger 记录请求耗时与状态码。
type AccessLogger struct{}

// Wrap 包装下一个处理器。
func (m AccessLogger) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		log.Printf("[http] %s %s %d %s", r.Method, r.URL.Path, recorder.status, time.Since(start).Round(time.Millisecond))
	})
}

// SecurityHeaders 设置通用安全响应头。
type SecurityHeaders struct{}

// Wrap 包装下一个处理器。
func (m SecurityHeaders) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("X-Frame-Options", "SAMEORIGIN")
		header.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// statusRecorder 记录响应状态码。
type statusRecorder struct {
	http.ResponseWriter
	status int
}

// WriteHeader 记录状态码后透传给底层 ResponseWriter。
func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
