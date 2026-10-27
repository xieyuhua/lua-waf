// Package redis 提供一个极简的 Redis（RESP2）客户端。
//
// 只实现日志消费需要的少数命令，避免引入第三方依赖：
// PING / AUTH / SELECT / BRPOPLPUSH / LREM / LLEN / RPUSH（测试用）。
package redis

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// 常见错误。
var (
	// ErrNil 表示命令返回空值（Redis 的 nil bulk replay）。
	ErrNil = errors.New("redis: nil reply")
	// ErrClosed 表示连接已关闭。
	ErrClosed = errors.New("redis: client closed")
)

// Options 是连接参数。
type Options struct {
	// Addr 为 host:port。
	Addr string
	// Password 为空表示不鉴权。
	Password string
	// DB 为库号，0 表示默认库。
	DB int
	// Timeout 为读写超时（含 BRPOPLPUSH 的最小等待）。
	Timeout time.Duration
}

// Client 是单连接的极简客户端（非并发安全，调用方需串行使用）。
type Client struct {
	conn   net.Conn
	reader *bufio.Reader
	opts   Options
	closed bool
}

// Dial 建立连接并完成 AUTH / SELECT。
func Dial(opts Options) (*Client, error) {
	if strings.TrimSpace(opts.Addr) == "" {
		opts.Addr = "127.0.0.1:6379"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Second
	}

	conn, err := net.DialTimeout("tcp", opts.Addr, opts.Timeout)
	if err != nil {
		return nil, fmt.Errorf("连接 Redis 失败: %w", err)
	}

	client := &Client{conn: conn, reader: bufio.NewReader(conn), opts: opts}
	if opts.Password != "" {
		if _, err := client.Do("AUTH", opts.Password); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("Redis 鉴权失败: %w", err)
		}
	}
	if opts.DB > 0 {
		if _, err := client.Do("SELECT", strconv.Itoa(opts.DB)); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("Redis 选择库失败: %w", err)
		}
	}
	return client, nil
}

// Close 关闭连接。
func (c *Client) Close() error {
	if c == nil || c.closed {
		return nil
	}
	c.closed = true
	return c.conn.Close()
}

// Ping 探活。
func (c *Client) Ping() error {
	_, err := c.Do("PING")
	return err
}

// LLen 返回列表长度。
func (c *Client) LLen(key string) (int64, error) {
	value, err := c.Do("LLEN", key)
	if err != nil {
		return 0, err
	}
	number, _ := value.(int64)
	return number, nil
}

// BRPopLPush 阻塞地从 source 取出一个元素并放入 destination，超时返回 ErrNil。
func (c *Client) BRPopLPush(source, destination string, timeout time.Duration) (string, error) {
	seconds := int(timeout / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	// 阻塞命令的读超时要比等待时间更长
	if err := c.conn.SetReadDeadline(time.Now().Add(timeout + c.opts.Timeout)); err != nil {
		return "", err
	}
	defer func() {
		_ = c.conn.SetReadDeadline(time.Now().Add(c.opts.Timeout))
	}()

	value, err := c.Do("BRPOPLPUSH", source, destination, strconv.Itoa(seconds))
	if err != nil {
		return "", err
	}
	return value.(string), nil
}

// LRem 从列表中删除指定元素。
func (c *Client) LRem(key string, count int, value string) error {
	_, err := c.Do("LREM", key, strconv.Itoa(count), value)
	return err
}

// RPop 从列表尾部取出一个元素。
func (c *Client) RPop(key string) (string, error) {
	value, err := c.Do("RPOP", key)
	if err != nil {
		return "", err
	}
	return value.(string), nil
}

// RPush 向列表尾部追加元素。
func (c *Client) RPush(key string, values ...string) error {
	args := make([]string, 0, len(values)+2)
	args = append(args, "RPUSH", key)
	args = append(args, values...)
	_, err := c.Do(args...)
	return err
}

// Do 执行一条命令并返回解析后的响应。
func (c *Client) Do(args ...string) (any, error) {
	if c == nil || c.closed {
		return nil, ErrClosed
	}
	if err := c.conn.SetWriteDeadline(time.Now().Add(c.opts.Timeout)); err != nil {
		return nil, err
	}
	if _, err := c.writeCommand(args); err != nil {
		return nil, err
	}
	if err := c.conn.SetReadDeadline(time.Now().Add(c.opts.Timeout)); err != nil {
		return nil, err
	}
	return c.readReply()
}

// writeCommand 按 RESP 数组格式写入命令。
func (c *Client) writeCommand(args []string) (int, error) {
	var builder strings.Builder
	builder.WriteString("*" + strconv.Itoa(len(args)) + "\r\n")
	for _, arg := range args {
		builder.WriteString("$" + strconv.Itoa(len(arg)) + "\r\n")
		builder.WriteString(arg)
		builder.WriteString("\r\n")
	}
	return c.conn.Write([]byte(builder.String()))
}

// readReply 解析一条响应。
func (c *Client) readReply() (any, error) {
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return nil, errors.New("redis: 空响应")
	}

	switch line[0] {
	case '+':
		return line[1:], nil
	case '-':
		return nil, errors.New(line[1:])
	case ':':
		return strconv.ParseInt(line[1:], 10, 64)
	case '$':
		length, err := strconv.Atoi(line[1:])
		if err != nil {
			return nil, err
		}
		if length < 0 {
			return nil, ErrNil
		}
		buffer := make([]byte, length+2)
		if _, err := readFull(c.reader, buffer); err != nil {
			return nil, err
		}
		return string(buffer[:length]), nil
	case '*':
		count, err := strconv.Atoi(line[1:])
		if err != nil {
			return nil, err
		}
		if count < 0 {
			return nil, ErrNil
		}
		items := make([]any, 0, count)
		for i := 0; i < count; i++ {
			item, err := c.readReply()
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		return items, nil
	default:
		return nil, fmt.Errorf("redis: 无法解析响应 %q", line)
	}
}

// readFull 读满缓冲区。
func readFull(reader *bufio.Reader, buffer []byte) (int, error) {
	read := 0
	for read < len(buffer) {
		n, err := reader.Read(buffer[read:])
		read += n
		if err != nil {
			return read, err
		}
	}
	return read, nil
}
