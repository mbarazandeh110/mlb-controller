package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mlb-controller/internal/adapters/logging"
	"mlb-controller/internal/di"
	logging_ports "mlb-controller/internal/ports/logging"
)

func main() {
	configPath := flag.String("config", "/etc/mlb-controller/config.yaml", "path to configuration file")
	flag.Parse()

	// ایجاد Context متصل به سیگنال‌های سیستم‌عامل برای Graceful Shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	container, err := di.NewContainer(*configPath)
	if err != nil {
		tempLogger, _ := logging.New(logging.LogConfig{Level: "info", Format: "json"})
		if tempLogger != nil {
			tempLogger.Fatal("Failed to initialize DI container", logging_ports.Field{Key: "error", Value: err})
		}
		os.Exit(1)
	}

	// مدیریت چرخه حیات (Lifecycle Management) و پاکسازی منابع با Timeout قطعی
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		container.Logger.Info("Initiating graceful shutdown sequence...")
		if stopErr := container.App.Stop(shutdownCtx); stopErr != nil {
			container.Logger.Error("Failed to stop app gracefully", logging_ports.Field{Key: "error", Value: stopErr})
		} else {
			container.Logger.Info("Application stopped successfully")
		}

		container.Logger.Sync()
	}()

	container.Logger.Info("Starting mlb-controller...")

	// پاس دادن Context کنترل‌شده با سیگنال به متد Start
	// با دریافت SIGTERM از سمت Kubelet، این Context کنسل شده و روتین‌های داخلی متوقف می‌شوند
	if err := container.App.Start(ctx); err != nil {
		container.Logger.Error("Application exited with error", logging_ports.Field{Key: "error", Value: err})
		os.Exit(1)
	}
}
