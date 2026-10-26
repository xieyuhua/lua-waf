package httpapi

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"wafadmin/internal/model"
)

// QueryParams 封装 URL 查询参数的解析，避免各 handler 重复写类型转换。
type QueryParams struct {
	values url.Values
}

// NewQueryParams 由请求构造查询参数助手。
func NewQueryParams(r *http.Request) QueryParams {
	return QueryParams{values: r.URL.Query()}
}

// Int 读取整型参数，缺失或非法时返回默认值。
func (p QueryParams) Int(key string, def int) int {
	raw := p.String(key)
	if raw == "" {
		return def
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return value
}

// Int64 读取 64 位整型参数。
func (p QueryParams) Int64(key string, def int64) int64 {
	raw := p.String(key)
	if raw == "" {
		return def
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return def
	}
	return value
}

// String 读取字符串参数并去除首尾空白。
func (p QueryParams) String(key string) string {
	return strings.TrimSpace(p.values.Get(key))
}

// Page 把查询参数转换为统一的分页条件。
func (p QueryParams) Page() model.ListQuery {
	return model.ListQuery{
		Page:    p.Int("page", 1),
		Size:    p.Int("size", 0),
		Keyword: p.String("keyword"),
	}
}

// PathParams 封装路径通配符参数（Go 1.22 路由）的读取。
type PathParams struct {
	Request *http.Request
}

// Int64 读取路径中的 64 位整型参数。
func (p PathParams) Int64(key string) int64 {
	raw := strings.TrimSpace(p.Request.PathValue(key))
	if raw == "" {
		return 0
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}
	return value
}

// String 读取路径中的字符串参数。
func (p PathParams) String(key string) string {
	return strings.TrimSpace(p.Request.PathValue(key))
}
