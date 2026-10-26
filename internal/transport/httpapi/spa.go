package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

// serveSPA 提供内嵌的前端静态资源；未命中的路径回落到 index.html，
// 以支持 Vue Router 的 history 模式。
func (s *Server) serveSPA(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}
	if s.Static == nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("前端资源未构建，请先执行：cd web && npm install && npm run build"))
		return
	}

	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" || name == "." {
		name = "index.html"
	}
	if info, ok := s.assetInfo(name); ok {
		s.writeAsset(w, r, name, info)
		return
	}

	// 非静态资源路径交给前端路由处理
	if info, ok := s.assetInfo("index.html"); ok {
		s.writeAsset(w, r, "index.html", info)
		return
	}
	http.NotFound(w, r)
}

// assetInfo 返回资源信息，不存在或为目录时返回 false。
func (s *Server) assetInfo(name string) (fs.FileInfo, bool) {
	file, err := s.Static.Open(name)
	if err != nil {
		return nil, false
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return nil, false
	}
	return info, true
}

// writeAsset 输出静态资源并设置缓存策略。
func (s *Server) writeAsset(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo) {
	data, err := fs.ReadFile(s.Static, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	contentType := mime.TypeByExtension(path.Ext(name))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)

	// 构建产物带内容哈希，可长期缓存；入口 HTML 必须每次校验。
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeContent(w, r, name, info.ModTime(), bytes.NewReader(data))
}

// frontendVersion 返回内嵌前端 index.html 的短指纹，用于确认浏览器加载的是哪一版构建。
func (s *Server) frontendVersion() string {
	if s.Static == nil {
		return ""
	}

	data, err := fs.ReadFile(s.Static, "index.html")
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)

	// index.html 只引用带哈希的资源名，其指纹即可代表整次构建
	return hex.EncodeToString(sum[:4])
}
