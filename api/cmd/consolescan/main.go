package main

import (
	"context"
	"encoding/json"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/local/mtn-fibrex/api/internal/config"
	"github.com/local/mtn-fibrex/api/internal/router"
)

func main() {
	out := flag.String("out", "console-scan.json", "path for the JSON scan result")
	maxPages := flag.Int("max-pages", 3000, "maximum number of console pages to request")
	pause := flag.Duration("pause", 150*time.Millisecond, "delay between page requests")
	flag.Parse()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	if cfg.RouterAdapter != "huawei-web" {
		logger.Error("console scan requires ROUTER_ADAPTER=huawei-web")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	adapter, err := router.NewHuaweiAdapter(cfg.RouterAddress, cfg.RouterInsecureTLS, cfg.RouterUsername, cfg.RouterPassword)
	if err != nil {
		logger.Error("configure router adapter", "error", err)
		os.Exit(1)
	}
	scan, err := adapter.ScanConsole(ctx, *maxPages, *pause)
	if err != nil {
		logger.Error("scan console", "error", err)
		os.Exit(1)
	}
	file, err := os.OpenFile(*out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		logger.Error("create scan output", "error", err)
		os.Exit(1)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(scan); err != nil {
		logger.Error("write scan output", "error", err)
		os.Exit(1)
	}
	logger.Info("console scan complete", "pages", len(scan.Pages), "missing", len(scan.Missing), "menuEntries", len(scan.Menu), "stopped", scan.Stopped, "out", *out)
}
