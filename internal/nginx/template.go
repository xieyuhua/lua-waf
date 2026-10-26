package nginx

// proxyTemplate 是站点 vhost 的渲染模板。
const proxyTemplate = `# ============================================================
# 本文件由 wafadmin 自动生成，请勿手工修改
# 站点: {{.SiteName}}
# 生成时间: {{.GeneratedAt}}
# 如需调整请登录 WAF 后台 -> 站点管理
# ============================================================
upstream {{.Upstream}} {
{{- range .BackendAddr}}
    server {{.}} max_fails=3 fail_timeout=10s;
{{- end}}
    keepalive 32;
}

server {
    listen      {{.Port}};
    server_name {{.SiteName}};
{{- if .SSLEnabled}}
    listen      {{.SSLPort}} ssl;
    ssl_certificate     {{.CertFile}};
    ssl_certificate_key {{.KeyFile}};
    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_ciphers         HIGH:!aNULL:!MD5:!RC4;
    ssl_session_cache   shared:SSL:10m;
    ssl_session_timeout 10m;
{{- end}}

    charset utf-8;
    # 放宽请求体上限，配合 WAF 的 POST 分段 / 畸形数据检测
    client_max_body_size 32m;
    client_body_buffer_size 256k;

    access_log {{.AccessLog}};
    error_log  {{.ErrorLog}} {{.DebugLevel}};
{{- if .HasUnreal}}

    # 诱饵地址：来源命中后直接丢弃请求，避免暴露真实后端
    if ($remote_addr ~ "^({{.UnrealPattern}})$") {
        return 404;
    }
{{- end}}

    location / {
        proxy_pass http://{{.Upstream}};
        proxy_http_version 1.1;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Connection        "";
        proxy_connect_timeout 5s;
        proxy_send_timeout    60s;
        proxy_read_timeout    60s;
        proxy_next_upstream   error timeout invalid_header http_502 http_503 http_504;
    }
}
`
