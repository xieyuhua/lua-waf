package ingest_test

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"wafadmin/internal/ingest"
	"wafadmin/internal/model"
	"wafadmin/internal/repository/memory"
	"wafadmin/pkg/redis"
)

// fakeRedis 是一个只支持列表命令的极简 RESP 服务端，用于端到端验证消费者。
type fakeRedis struct {
	mu    sync.Mutex
	lists map[string][]string
	ln    net.Listener
}

func newFakeRedis(t *testing.T) *fakeRedis {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	server := &fakeRedis{lists: make(map[string][]string), ln: listener}
	go server.serve()
	return server
}

func (f *fakeRedis) addr() string { return f.ln.Addr().String() }
func (f *fakeRedis) close()       { _ = f.ln.Close() }

func (f *fakeRedis) serve() {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.handle(conn)
	}
}

func (f *fakeRedis) handle(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)

	for {
		args, err := readCommand(reader)
		if err != nil {
			return
		}
		if len(args) == 0 {
			continue
		}

		switch strings.ToUpper(args[0]) {
		case "PING":
			fmt.Fprint(conn, "+PONG\r\n")
		case "RPUSH":
			f.mu.Lock()
			key := args[1]
			f.lists[key] = append(f.lists[key], args[2:]...)
			length := len(f.lists[key])
			f.mu.Unlock()
			fmt.Fprintf(conn, ":%d\r\n", length)
		case "LLEN":
			f.mu.Lock()
			length := len(f.lists[args[1]])
			f.mu.Unlock()
			fmt.Fprintf(conn, ":%d\r\n", length)
		case "RPOP":
			f.mu.Lock()
			key := args[1]
			items := f.lists[key]
			if len(items) == 0 {
				f.mu.Unlock()
				fmt.Fprint(conn, "$-1\r\n")
				continue
			}
			value := items[0]
			f.lists[key] = items[1:]
			f.mu.Unlock()
			writeBulk(conn, value)
		case "BRPOPLPUSH":
			// 简化：非阻塞语义（有数据立即返回，没有则等待短暂时间后返回 nil）
			source, dest := args[1], args[2]
			f.mu.Lock()
			items := f.lists[source]
			if len(items) == 0 {
				f.mu.Unlock()
				time.Sleep(50 * time.Millisecond)
				fmt.Fprint(conn, "$-1\r\n")
				continue
			}
			value := items[0]
			f.lists[source] = items[1:]
			f.lists[dest] = append(f.lists[dest], value)
			f.mu.Unlock()
			writeBulk(conn, value)
		case "LREM":
			key := args[1]
			value := args[3]
			f.mu.Lock()
			items := f.lists[key]
			removed := 0
			kept := items[:0]
			for _, item := range items {
				if removed == 0 && item == value {
					removed++
					continue
				}
				kept = append(kept, item)
			}
			f.lists[key] = kept
			f.mu.Unlock()
			fmt.Fprintf(conn, ":%d\r\n", removed)
		default:
			fmt.Fprintf(conn, "-ERR unsupported %s\r\n", args[0])
		}
	}
}

// readCommand 读取一条 RESP 数组命令。
func readCommand(reader *bufio.Reader) ([]string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimRight(line, "\r\n")
	if !strings.HasPrefix(line, "*") {
		return nil, fmt.Errorf("非数组命令: %q", line)
	}
	count, err := strconv.Atoi(line[1:])
	if err != nil {
		return nil, err
	}
	args := make([]string, 0, count)
	for i := 0; i < count; i++ {
		header, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		length, err := strconv.Atoi(strings.TrimRight(header, "\r\n")[1:])
		if err != nil {
			return nil, err
		}
		buffer := make([]byte, length+2)
		if _, err := reader.Read(buffer); err != nil {
			return nil, err
		}
		args = append(args, string(buffer[:length]))
	}
	return args, nil
}

// writeBulk 输出一条 bulk 字符串。
func writeBulk(conn net.Conn, value string) {
	fmt.Fprintf(conn, "$%d\r\n%s\r\n", len(value), value)
}

// TestConsumerWritesToRepository 校验从 Redis 取出的日志被批量写入仓储并移出处理中队列。
func TestConsumerWritesToRepository(t *testing.T) {
	server := newFakeRedis(t)
	defer server.close()

	client, err := redis.Dial(redis.Options{Addr: server.addr()})
	if err != nil {
		t.Fatalf("连接假 Redis 失败: %v", err)
	}
	// 预置 3 条日志
	records := []string{
		`{"client_ip":"1.1.1.1","local_time":"2026-10-27 10:00:00","attack_type":"SQL注入","req_url":"/a?id=1","area":"中国-北京","rule_tag":"args->sql"}`,
		`{"client_ip":"2.2.2.2","local_time":"2026-10-27 10:00:01","attack_type":"CC攻击","req_url":"/b","area":"美国","rule_tag":"cc"}`,
		`not-a-json`,
	}
	for _, raw := range records {
		if err := client.RPush("waf:logs", raw); err != nil {
			t.Fatalf("写入队列失败: %v", err)
		}
	}
	_ = client.Close()

	store := (&memory.Store{}).Init(true)
	consumer := ingest.New(ingest.Options{
		Redis:  redis.Options{Addr: server.addr()},
		Source: "waf:logs",
		Batch:  10,
		Block:  100 * time.Millisecond,
	}, store.Repos().Log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go consumer.Run(ctx)

	// 等待消费完成（以消费者统计为准，演示数据本身已有日志）
	deadline := time.Now().Add(5 * time.Second)
	var stats ingest.Stats
	for time.Now().Before(deadline) {
		stats = consumer.Stats()
		if stats.Written >= 2 && stats.ProcessingLength == 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	stats = consumer.Stats()
	if stats.Consumed < 3 {
		t.Fatalf("应消费 3 条，实际 %d（stats=%+v）", stats.Consumed, stats)
	}
	if stats.Written < 2 {
		t.Fatalf("应写入 2 条有效日志，实际 %d（stats=%+v）", stats.Written, stats)
	}
	if stats.Failed < 1 {
		t.Fatalf("应记录 1 条解析失败，实际 %d", stats.Failed)
	}
	if stats.ProcessingLength != 0 {
		t.Fatalf("处理中队列应已清空，实际 %d", stats.ProcessingLength)
	}

	items, _, err := store.Repos().Log.List(ctx, model.LogQuery{ListQuery: model.ListQuery{Page: 1, Size: 5}})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	found := false
	for _, item := range items {
		if item.ClientIp == "1.1.1.1" && item.AttackType == "SQL注入" {
			found = true
		}
	}
	if !found {
		t.Fatal("新增日志未写入仓储")
	}
}
