-- ============================================================
-- waf_log 表结构（openresty 侧异步写入，后台只读）
-- 与 lua-waf/log_mysql.lua 的 INSERT 列一一对应，
-- 也与后台 internal/model/waflog.go 的字段保持一致。
--
-- 使用方式（mysql / redis 日志模式必需）：
--   mysql -u root -p waf < lua-waf/sql/waf_log.sql
-- ============================================================
CREATE TABLE IF NOT EXISTS `waf_log` (
  `id`          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `client_ip`   VARCHAR(64)     NOT NULL DEFAULT ''  COMMENT '来源 IP',
  `local_time`  VARCHAR(32)     NOT NULL DEFAULT ''  COMMENT '本地时间 YYYY-MM-DD HH:MM:SS',
  `server_name` VARCHAR(190)    NOT NULL DEFAULT ''  COMMENT '命中的站点域名',
  `user_agent`  VARCHAR(512)    NOT NULL DEFAULT ''  COMMENT 'User-Agent',
  `attack_type` VARCHAR(64)     NOT NULL DEFAULT ''  COMMENT '攻击类型',
  `req_url`     VARCHAR(1024)   NOT NULL DEFAULT ''  COMMENT '请求 URL',
  `area`        VARCHAR(64)     NOT NULL DEFAULT ''  COMMENT 'IP 归属地',
  `req_data`    TEXT                                 COMMENT '请求体 / 命中内容',
  `rule_tag`    VARCHAR(190)    NOT NULL DEFAULT ''  COMMENT '命中的规则标签',
  PRIMARY KEY (`id`),
  KEY `idx_local_time`  (`local_time`),
  KEY `idx_client_ip`   (`client_ip`),
  KEY `idx_attack_type` (`attack_type`),
  KEY `idx_server_name` (`server_name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='WAF 攻击日志';
