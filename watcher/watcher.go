package watcher

import (
	"context"
	"log"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

type Callback func(event fsnotify.Event)

func Watch(ctx context.Context, path string, logger *zap.Logger, callback Callback) {
	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	go func() {
		defer func() { cancel() }()
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				logger.Debug("file system event", zap.String("path", event.Name), zap.String("op", event.Op.String()))
				go callback(event)
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				logger.Error("file system watch error", zap.String("path", path), zap.Error(err))
			}
		}
	}()

	err = watcher.Add(path)
	if err != nil {
		logger.Fatal("failed to watch file", zap.String("path", path), zap.Error(err))
		cancel()
	}

	logger.Info("watching file", zap.String("path", path))
	<-watchCtx.Done()
	logger.Info("file watcher stopped", zap.String("path", path))
}
