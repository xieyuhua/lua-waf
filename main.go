package main

import (
	"context"
	"embed"
	"flag"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"wafadmin/internal/app"
	"wafadmin/internal/config"
)

// 构建时可通过 -ldflags "-X main.version=x.y.z" 覆盖版本号。
var version = "2.0.0"

// webAssets 内嵌 Vue 前端构建产物，保证单文件分发。
//
//go:embed all:web/dist
var webAssets embed.FS

func main() {
	configPath := flag.String("config", "", "配置文件路径，默认取 WAF_CONFIG 或 conf/app.ini")
	flag.Parse()

	static, err := fs.Sub(webAssets, "web/dist")
	if err != nil {
		log.Fatalf("[fatal] 加载内嵌前端资源失败: %v", err)
	}

	instance, err := app.New(app.Options{
		Config:  &config.Loader{Path: *configPath},
		Static:  static,
		Version: version,
	})
	if err != nil {
		log.Fatalf("[fatal] 启动失败: %v", err)
	}

	go func() {
		if runErr := instance.Run(); runErr != nil {
			log.Printf("[fatal] 服务异常退出: %v", runErr)
			os.Exit(1)
		}
	}()

	// 等待退出信号后优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[info] 正在关闭服务...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := instance.Shutdown(ctx); err != nil {
		log.Printf("[warn] 关闭服务时出现错误: %v", err)
	}
	log.Println("[info] 服务已停止")
}
