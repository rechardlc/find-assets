package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/find-assets/scanner/internal/exporter"
	"github.com/find-assets/scanner/internal/server"
	"github.com/find-assets/scanner/internal/service"
	"github.com/find-assets/scanner/internal/source"
)

func main() {
	var (
		mode      = flag.String("mode", "day", "选股模式：day | week （CLI 模式必填）")
		workers   = flag.Int("workers", 100, "最大并发数")
		barsLimit = flag.Int("bars", 600, "拉取日线根数")
		rangeArg  = flag.Float64("range", 1.5, "[day 策略] 均线粘合度阈值，百分比单位，默认 1.5 (=1.5%)")
		cohesion  = flag.Float64("cohesion", 0, "[day 策略] 粘合度阈值（小数，已废弃，建议使用 -range）")
		exportArg = flag.String("export", "console", "导出格式列表，逗号分隔：console,json,md")
		outDir    = flag.String("out", "./output", "导出文件输出目录")
		serve     = flag.Bool("serve", false, "以 HTTP 服务模式运行")
		addr      = flag.String("addr", ":8080", "HTTP 监听地址")
	)
	flag.Parse()

	src := source.NewEastMoney()
	svc := service.New(src)

	if *serve {
		runServer(svc, *addr)
		return
	}

	if *mode == "" {
		fmt.Fprintln(os.Stderr, "请使用 -mode=day 或 -mode=week 指定策略；或 -serve 启动 HTTP 服务")
		flag.Usage()
		os.Exit(2)
	}

	runCLI(svc, *mode, *workers, *barsLimit, *rangeArg, *cohesion, *exportArg, *outDir)
}

// ---------------- CLI 模式 ----------------

func runCLI(svc *service.ScanService, mode string, workers, bars int, rangePct, cohesion float64, exportArg, outDir string) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Printf("★ 选股器启动，当前运行模式: %s 线策略 ★\n", mode)
	if mode == "day" {
		if cohesion > 0 {
			fmt.Printf("   均线粘合度阈值: %.4f (cohesion)\n", cohesion)
		} else {
			fmt.Printf("   均线粘合度阈值: %.2f%% (range)\n", rangePct)
		}
	}
	fmt.Println("1. 正在拉取全市场股票清单...")

	var totalStocks int
	var nextProgress int64 = 1000

	rep, err := svc.Run(ctx, service.Params{
		Mode: mode, Workers: workers, BarsLimit: bars,
		Range: rangePct, Cohesion: cohesion, TaskID: uuid.NewString(),
		OnStocks: func(total int) {
			totalStocks = total
			fmt.Printf("成功获取到 %d 只股票，开始并发扫描...\n", total)
		},
		Progress: func(done, total int64) {
			threshold := atomic.LoadInt64(&nextProgress)
			if done >= threshold {
				if atomic.CompareAndSwapInt64(&nextProgress, threshold, threshold+1000) {
					fmt.Printf("已扫描 %d/%d 只股票...\n", done, total)
				}
			}
		},
	})
	if err != nil {
		log.Fatalf("扫描失败: %v", err)
	}
	_ = totalStocks

	formats := splitFormats(exportArg)
	if err := dispatchExports(rep, formats, outDir, mode); err != nil {
		log.Fatalf("导出失败: %v", err)
	}
}

func splitFormats(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		out = append(out, "console")
	}
	return out
}

func dispatchExports(rep *exporter.Report, formats []string, outDir, mode string) error {
	needFile := false
	for _, f := range formats {
		if f != "console" {
			needFile = true
			break
		}
	}
	if needFile {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return err
		}
	}
	ts := rep.StartedAt.Format("20060102_150405")
	for _, f := range formats {
		exp := exporter.Get(f)
		if exp == nil {
			fmt.Fprintf(os.Stderr, "未知导出格式: %s\n", f)
			continue
		}
		if f == "console" {
			if err := exp.Write(os.Stdout, rep); err != nil {
				return err
			}
			continue
		}
		path := filepath.Join(outDir, fmt.Sprintf("scan_%s_%s.%s", mode, ts, f))
		fp, err := os.Create(path)
		if err != nil {
			return err
		}
		err = exp.Write(fp, rep)
		_ = fp.Close()
		if err != nil {
			return err
		}
		fmt.Printf("已导出 %s -> %s\n", f, path)
	}
	return nil
}

// ---------------- HTTP 服务模式 ----------------

func runServer(svc *service.ScanService, addr string) {
	router, handler := server.NewRouter(svc)
	srv := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		fmt.Printf("★ HTTP 服务已启动: http://%s/api/v1 ★\n", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("服务启动失败: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	fmt.Println("\n正在关闭服务...")
	handler.Shutdown()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
