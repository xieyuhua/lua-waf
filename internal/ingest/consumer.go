// Package ingest 实现「lua 只写 Redis，Go 批量入库」的日志消费链路。
//
// 链路：openresty(config_log_mode="redis") --RPUSH--> Redis List --BRPOPLPUSH--> 本消费者
//
//	--批量 INSERT--> MySQL waf_log --LREM--> 处理中队列
//
// 语义：at-least-once。取出的元素先进入 processing 列表，入库成功才删除；
// 进程崩溃或入库失败时元素仍留在 processing，重启后自动回灌到源队列，不会丢日志。
package ingest

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"wafadmin/internal/model"
	"wafadmin/internal/repository"
	"wafadmin/pkg/redis"
)

// 默认参数。
const (
	defaultBatch = 200
	defaultBlock = 2 * time.Second
	maxBackoff   = 30 * time.Second
)

// Options 是消费者配置。
type Options struct {
	// Redis 连接参数。
	Redis redis.Options
	// Source 为源队列键（lua RPUSH 的列表）。
	Source string
	// Processing 为处理中队列键（用于崩溃恢复）。
	Processing string
	// Batch 为单批最大条数。
	Batch int
	// Block 为单次阻塞等待时长。
	Block time.Duration
}

// Stats 是消费统计。
type Stats struct {
	// Enabled 表示消费者是否在运行。
	Enabled bool `json:"enabled"`
	// Connected 表示当前 Redis 连接是否正常。
	Connected bool `json:"connected"`
	// Source / Processing 队列键。
	Source     string `json:"source"`
	Processing string `json:"processing"`
	// QueueLength 为源队列当前长度。
	QueueLength int64 `json:"queue_length"`
	// ProcessingLength 为处理中队列长度。
	ProcessingLength int64 `json:"processing_length"`
	// Consumed / Written / Failed 累计计数。
	Consumed int64 `json:"consumed"`
	Written  int64 `json:"written"`
	Failed   int64 `json:"failed"`
	// LastError 为最近一次错误。
	LastError string `json:"last_error"`
	// LastFlushAt 为最近一次成功入库时间。
	LastFlushAt string `json:"last_flush_at"`
}

// Consumer 是 Redis → MySQL 的日志消费者。
type Consumer struct {
	// Opts 为运行参数。
	Opts Options
	// Logs 为目标仓储。
	Logs repository.LogRepository
	// Logf 为可选的日志输出函数。
	Logf func(format string, args ...any)

	mu        sync.Mutex
	consumed  int64
	written   int64
	failed    int64
	lastError string
	lastFlush time.Time
	connected bool
	enabled   bool
	queueLen  int64
	procLen   int64
}

// New 构造消费者并补默认值。
func New(opts Options, logs repository.LogRepository) *Consumer {
	if opts.Source == "" {
		opts.Source = "waf:logs"
	}
	if opts.Processing == "" {
		opts.Processing = opts.Source + ":processing"
	}
	if opts.Batch <= 0 {
		opts.Batch = defaultBatch
	}
	if opts.Block <= 0 {
		opts.Block = defaultBlock
	}
	return &Consumer{Opts: opts, Logs: logs}
}

// Run 启动消费循环，ctx 结束时返回。
func (c *Consumer) Run(ctx context.Context) {
	c.setEnabled(true)
	defer c.setEnabled(false)

	// 启动时先探一次连通性：把「没开 / 连不上 / 口令错」这类原因第一时间讲清楚，
	// 否则表现就是「Redis 里日志一直不被消费」，很难定位。
	c.probeRedis()

	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}

		client, err := redis.Dial(c.Opts.Redis)
		if err != nil {
			c.setError(err)
			c.sleep(ctx, backoff)
			backoff = nextBackoff(backoff)
			continue
		}
		c.setConnected(true)
		c.recoverProcessing(ctx, client)
		backoff = time.Second

		// 内层循环：连接可用时持续消费
		for ctx.Err() == nil {
			if err := c.consumeOnce(ctx, client); err != nil {
				if errors.Is(err, redis.ErrClosed) || isNetworkError(err) {
					c.setError(err)
					break
				}
				c.setError(err)
				c.sleep(ctx, time.Second)
			}
			c.refreshLengths(client)
		}

		c.setConnected(false)
		_ = client.Close()
	}
}

// probeRedis 启动自检：连接并 PING 源队列所在的 Redis，把结果写进日志。
func (c *Consumer) probeRedis() {
	client, err := redis.Dial(c.Opts.Redis)
	if err != nil {
		c.setError(err)
		c.logf("[error] 日志入库消费者无法连接 Redis（%s db=%d）：%v；队列 %s 里的日志不会被消费",
			c.Opts.Redis.Addr, c.Opts.Redis.DB, err, c.Opts.Source)
		return
	}
	defer func() { _ = client.Close() }()

	if err := client.Ping(); err != nil {
		c.setError(err)
		c.logf("[error] 日志入库消费者 Redis PING 失败（%s db=%d）：%v；口令或权限可能不对，队列 %s 里的日志不会被消费",
			c.Opts.Redis.Addr, c.Opts.Redis.DB, err, c.Opts.Source)
		return
	}

	c.setConnected(true)
	c.refreshLengths(client)
	c.logf("[info] 日志入库消费者已就绪：Redis %s db=%d，源队列 %s（当前积压 %d 条），处理中队列 %s，每批 %d 条",
		c.Opts.Redis.Addr, c.Opts.Redis.DB, c.Opts.Source, c.queueLen, c.Opts.Processing, c.Opts.Batch)
}

// consumeOnce 取一批并入库一次。
func (c *Consumer) consumeOnce(ctx context.Context, client *redis.Client) error {
	items := make([]string, 0, c.Opts.Batch)
	for len(items) < c.Opts.Batch {
		value, err := client.BRPopLPush(c.Opts.Source, c.Opts.Processing, c.Opts.Block)
		if err != nil {
			if errors.Is(err, redis.ErrNil) {
				break
			}
			return err
		}
		items = append(items, value)
	}
	if len(items) == 0 {
		return nil
	}

	records := make([]model.WafLog, 0, len(items))
	invalid := make([]string, 0, 2)
	for _, raw := range items {
		record, ok := parseRecord(raw)
		if !ok {
			invalid = append(invalid, raw)
			continue
		}
		records = append(records, record)
	}

	c.addConsumed(int64(len(items)))

	// 无法解析的记录直接丢弃（避免卡住队列），计入失败
	for _, raw := range invalid {
		_ = client.LRem(c.Opts.Processing, 1, raw)
		c.addFailed(1)
		c.logf("[ingest] 丢弃无法解析的日志记录（长度 %d）", len(raw))
	}
	if len(records) == 0 {
		return nil
	}

	if _, err := c.Logs.BatchCreate(ctx, records); err != nil {
		// 入库失败：记录仍留在 processing，下一轮重试（超过阈值再告警）
		if hint := explainInsertError(err); hint != "" {
			c.logf("[error] 日志批量入库失败：%v %s", err, hint)
		}
		// 入库失败：记录仍留在 processing，下一轮重试（超过阈值再告警）
		c.addFailed(int64(len(records)))
		return err
	}

	for _, raw := range items {
		if !containsRecord(raw, records) {
			continue
		}
		_ = client.LRem(c.Opts.Processing, 1, raw)
	}
	c.addWritten(int64(len(records)))
	c.markFlushed()
	return nil
}

// recoverProcessing 把上次残留的处理中记录回灌到源队列（进程崩溃/重启场景）。
func (c *Consumer) recoverProcessing(ctx context.Context, client *redis.Client) {
	for ctx.Err() == nil {
		value, err := client.RPop(c.Opts.Processing)
		if err != nil {
			return
		}
		if strings.TrimSpace(value) == "" {
			continue
		}
		if err := client.RPush(c.Opts.Source, value); err != nil {
			c.setError(err)
			return
		}
	}
}

// refreshLengths 刷新队列长度指标。
func (c *Consumer) refreshLengths(client *redis.Client) {
	queue, err := client.LLen(c.Opts.Source)
	if err == nil {
		c.mu.Lock()
		c.queueLen = queue
		c.mu.Unlock()
	}
	proc, err := client.LLen(c.Opts.Processing)
	if err == nil {
		c.mu.Lock()
		c.procLen = proc
		c.mu.Unlock()
	}
}

// Stats 返回当前统计。
func (c *Consumer) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()

	stats := Stats{
		Enabled:          c.enabled,
		Connected:        c.connected,
		Source:           c.Opts.Source,
		Processing:       c.Opts.Processing,
		QueueLength:      c.queueLen,
		ProcessingLength: c.procLen,
		Consumed:         c.consumed,
		Written:          c.written,
		Failed:           c.failed,
		LastError:        c.lastError,
	}
	if !c.lastFlush.IsZero() {
		stats.LastFlushAt = c.lastFlush.Format("2006-01-02 15:04:05")
	}
	return stats
}

// nextBackoff 计算退避时间。
func nextBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next > maxBackoff {
		next = maxBackoff
	}
	return next
}

// sleep 可被 ctx 取消的等待。
func (c *Consumer) sleep(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// isNetworkError 判断是否网络层错误（需要重连）。
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	for _, keyword := range []string{"eof", "reset", "broken pipe", "connection", "timeout", "i/o"} {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

func (c *Consumer) setEnabled(value bool) {
	c.mu.Lock()
	c.enabled = value
	c.mu.Unlock()
}

func (c *Consumer) setConnected(value bool) {
	c.mu.Lock()
	c.connected = value
	c.mu.Unlock()
}

func (c *Consumer) setError(err error) {
	if err == nil {
		return
	}
	c.mu.Lock()
	c.lastError = err.Error()
	c.mu.Unlock()
	c.logf("[ingest] %v", err)
}

func (c *Consumer) addConsumed(n int64) {
	c.mu.Lock()
	c.consumed += n
	c.mu.Unlock()
}

func (c *Consumer) addWritten(n int64) {
	c.mu.Lock()
	c.written += n
	c.mu.Unlock()
}

func (c *Consumer) addFailed(n int64) {
	c.mu.Lock()
	c.failed += n
	c.mu.Unlock()
}

func (c *Consumer) markFlushed() {
	c.mu.Lock()
	c.lastFlush = time.Now()
	c.mu.Unlock()
}

func (c *Consumer) logf(format string, args ...any) {
	if c.Logf != nil {
		c.Logf(format, args...)
	}
}
