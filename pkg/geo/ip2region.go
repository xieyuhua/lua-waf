package geo

// 本文件实现 ip2region（v1 .db 格式）的读取，用于在**后端**把来源 IP 解析成归属地文本，
// 从而让 lua 侧不再为每个 worker 各加载一份 12MB 的 ip2region.db。
//
// 文件格式（与 lua-waf/ip2region.lua 完全一致）：
//   offset 0：int32 LE  firstIndexPtr
//   offset 4：int32 LE  lastIndexPtr
//   索引区：每 12 字节一块 —— [startIP uint32 LE][endIP uint32 LE][dataPtr uint32 LE]
//   数据区：dataPtr 处 [1 字节长度][region 字符串]，region 形如 "中国|0|广东省|深圳市|电信"
//
// 归属地文本的拼装规则与 lua 侧保持一致：国家 + 省 + 市 + 运营商，跳过 "0" 占位，
// 例如 "中国" + "重庆" + "重庆" + "电信" = "中国重庆重庆电信"（保证历史数据语义不变）。

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
)

// indexBlockLength 为单条索引长度。
const indexBlockLength = 12

// IP2Region 是 ip2region 数据文件的只读内存索引。
type IP2Region struct {
	data        []byte
	firstIndex  int
	totalBlocks int
}

// OpenIP2Region 把数据文件读入内存并建立索引。
//
// 单个文件 12MB 左右，读一次常驻；解析失败时调用方应把 error 当作「未启用归属地解析」处理。
func OpenIP2Region(path string) (*IP2Region, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("ip2region 数据库路径为空")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 ip2region 数据库失败: %w", err)
	}
	if len(data) < 8+indexBlockLength {
		return nil, fmt.Errorf("ip2region 数据库文件过小（%d 字节），可能不是有效数据文件", len(data))
	}

	firstIndex := int(binary.LittleEndian.Uint32(data[0:4]))
	lastIndex := int(binary.LittleEndian.Uint32(data[4:8]))
	if firstIndex <= 0 || lastIndex < firstIndex || lastIndex+indexBlockLength > len(data) {
		return nil, fmt.Errorf("ip2region 索引不合法（first=%d last=%d size=%d）", firstIndex, lastIndex, len(data))
	}

	totalBlocks := (lastIndex-firstIndex)/indexBlockLength + 1
	return &IP2Region{data: data, firstIndex: firstIndex, totalBlocks: totalBlocks}, nil
}

// Search 解析 IPv4 归属地，返回形如 "中国重庆重庆电信" 的文本。
func (r *IP2Region) Search(ip string) (string, bool) {
	if r == nil || len(r.data) == 0 {
		return "", false
	}

	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return "", false
	}
	ipv4 := parsed.To4()
	if ipv4 == nil {
		// IPv6 不在 ip2region 的覆盖范围内
		return "", false
	}

	target := binary.BigEndian.Uint32(ipv4)

	low, high := 0, r.totalBlocks
	for low <= high {
		mid := (low + high) / 2
		offset := r.firstIndex + mid*indexBlockLength
		if offset+indexBlockLength > len(r.data) {
			return "", false
		}

		startIP := binary.LittleEndian.Uint32(r.data[offset : offset+4])
		if target < startIP {
			high = mid - 1
			continue
		}

		endIP := binary.LittleEndian.Uint32(r.data[offset+4 : offset+8])
		if target > endIP {
			low = mid + 1
			continue
		}

		dataPtr := int(binary.LittleEndian.Uint32(r.data[offset+8 : offset+12]))
		return r.readRegion(dataPtr)
	}

	return "", false
}

// readRegion 从数据区读出 region 文本并拼装成归属地。
func (r *IP2Region) readRegion(dataPtr int) (string, bool) {
	if dataPtr <= 0 || dataPtr >= len(r.data) {
		return "", false
	}

	length := int(r.data[dataPtr])
	start := dataPtr + 1
	end := start + length
	if end > len(r.data) || length == 0 {
		return "", false
	}

	region := string(r.data[start:end])
	area := FormatIP2Region(region)
	if area == "" {
		return "", false
	}
	return area, true
}

// FormatIP2Region 把 "国家|区域|省|市|运营商" 拼成 "国家省市运营商"（跳过 "0" 占位）。
//
// 与 lua 侧 parse_ipinfo 的拼装规则保持一致，保证历史数据的 area 语义不变。
func FormatIP2Region(region string) string {
	parts := strings.Split(region, "|")
	if len(parts) < 5 {
		return ""
	}

	var builder strings.Builder
	for _, index := range []int{0, 2, 3, 4} {
		value := strings.TrimSpace(parts[index])
		if value == "" || value == "0" {
			continue
		}
		builder.WriteString(value)
	}
	return builder.String()
}

// resolverOnce / defaultRegion 保证进程内只加载一次数据文件。
var (
	resolverOnce  sync.Once
	defaultRegion *IP2Region
	resolverErr   error
)

// DefaultIP2Region 打开默认位置的数据文件（多次调用只读一次）。
//
// 找不到文件时返回 error，调用方据此跳过归属地解析（不影响日志入库）。
func DefaultIP2Region(path string) (*IP2Region, error) {
	resolverOnce.Do(func() {
		defaultRegion, resolverErr = OpenIP2Region(path)
	})
	return defaultRegion, resolverErr
}
