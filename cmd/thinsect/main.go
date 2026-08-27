// 火山岩薄片矿物边界复核台入口。
//
// 用法：
//
//	thinsect --addr :8080 --db ./thinsect.db      # 启动 HTTP 服务
//	thinsect --db ./smoke.db --smoke-test         # 跑端到端自检（Docker 验收契约）
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"task234-thinsect/internal/httpapi"
	"task234-thinsect/internal/service"
	"task234-thinsect/internal/smoke"
	"task234-thinsect/internal/store"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP 监听地址")
	dbPath := flag.String("db", "./thinsect.db", "SQLite 数据库文件路径")
	smokeTest := flag.Bool("smoke-test", false, "执行端到端自检后退出")
	flag.Parse()

	if *smokeTest {
		if err := smoke.Run(*dbPath); err != nil {
			fmt.Fprintln(os.Stderr, "SMOKE FAIL:", err)
			os.Exit(1)
		}
		return
	}

	db, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	app := service.New(db)
	srv := &http.Server{
		Addr:              *addr,
		Handler:           httpapi.New(app),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("火山岩薄片矿物边界复核台 listening on %s (db=%s)", *addr, *dbPath)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}
