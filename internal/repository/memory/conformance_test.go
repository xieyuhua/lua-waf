package memory_test

import (
	"testing"

	"wafadmin/internal/repository/memory"
	"wafadmin/internal/repository/repositorytest"
)

// TestConformance 用统一套件校验内存仓储的实现语义（与 mysql 实现共享同一套断言）。
func TestConformance(t *testing.T) {
	store := (&memory.Store{}).Init(false) // 不载入演示数据，避免干扰断言
	repositorytest.RunAll(t, store.Repos(), repositorytest.Options{
		Prefix: "mem-conf",
		// 内存实现没有大表问题，日志仓储一起校验
	})
}
