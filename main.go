package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/heilerich/dhcpd-coredns/backend"
	"github.com/heilerich/dhcpd-coredns/backend/etcd"
	"github.com/heilerich/dhcpd-coredns/config"
	"github.com/heilerich/dhcpd-coredns/util"
	"github.com/heilerich/dhcpd-coredns/watcher"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	atomic := zap.NewAtomicLevel()
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	logger := zap.New(zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderCfg),
		zapcore.Lock(os.Stdout),
		atomic,
	))
	defer logger.Sync()

	cfg := initConfig(logger)

	logLevel, err := zapcore.ParseLevel(cfg.LogLevel)
	if err != nil {
		logger.Fatal("invalid log level", zap.Error(err))
	}

	atomic.SetLevel(logLevel)

	shutdownWg := &util.TimeoutGroup{}

	etcdBackend, err := etcd.NewEtcdBackend(cfg, logger)
	if err != nil {
		logger.Fatal("failed to init etcd backend", zap.Error(err))
	}

	onStart := func() {
		shutdownWg.Add(1)
	}
	onStop := func() {
		cancel()
		shutdownWg.Done()
	}

	syncFn := watcher.CoordinateWatcher(ctx, cfg.Lease.File, etcdBackend, logger, onStart, onStop)

	go func() {
		if err := backend.RunCleaner(ctx, etcdBackend, syncFn, cfg); err != nil {
			logger.Warn("backend cleaning failed", zap.Error(err))
		}
		logger.Info("backend cleaner stopped")
		cancel()
		shutdownWg.Done()
	}()

	<-ctx.Done()

	shutdownCtx := context.Background()
	logger.Info("waiting for all routines to stop")
	if err := shutdownWg.WaitWithTimeout(shutdownCtx, 10*time.Second); err != nil {
		logger.Warn("orderly shutdown failed, terminating", zap.Error(err))
	}
	logger.Info("exit")
}

func initConfig(logger *zap.Logger) *config.Config {
	vp := viper.New()

	config.SetDefaults(vp)

	vp.SetConfigName("config")
	vp.SetConfigType("yaml")
	vp.AddConfigPath("/etc/dhcpd-coredns")

	configPath := pflag.StringP("config", "c", "", "config path")
	pflag.Parse()

	if *configPath != "" {
		vp.SetConfigFile(*configPath)
	}

	if err := vp.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			logger.Info("no configuration file at /etc/dhcpd-coredns/config.yaml")
		} else {
			logger.Fatal("failed to read config", zap.Error(err))
		}
	} else {
		logger.Info("read configuration file", zap.String("path", vp.ConfigFileUsed()))
	}

	var config config.Config
	vp.Unmarshal(&config)
	return &config
}
