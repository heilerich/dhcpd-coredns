package watcher

import (
	"context"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/heilerich/dhcpd-coredns/backend"
	"github.com/heilerich/dhcpd-coredns/parser"
	"go.uber.org/zap"
)

type coordinator struct {
	logger *zap.Logger
	mu     sync.Mutex
	queue  int
	signal chan struct{}
	ctx    context.Context
}

func NewCoordinator(ctx context.Context, logger *zap.Logger) *coordinator {
	return &coordinator{
		logger: logger,
		signal: make(chan struct{}),
		ctx:    ctx,
	}
}

func (c *coordinator) Signal() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.queue >= 1 {
		return
	}

	c.queue += 1
	go func() {
		select {
		case <-c.ctx.Done():
			return
		case c.signal <- struct{}{}:
			return
		}
	}()
	c.logger.Debug("coordinator signalled", zap.Int("queue", c.queue))
}

func (c *coordinator) Run(callback func()) {
	for {
		select {
		case <-c.ctx.Done():
			c.logger.Info("job coordinator stopped", zap.Error(c.ctx.Err()))
			c.ctx = nil
			return
		case <-c.signal:
			for {
				c.mu.Lock()
				if c.queue <= 0 {
					c.mu.Unlock()
					break
				}
				c.queue -= 1
				c.mu.Unlock()
				callback()
			}
		}
	}
}

func CoordinateWatcher(ctx context.Context, leaseFile string, backend backend.Backend, logger *zap.Logger, jobStart func(), jobStop func()) func(context.Context) {
	coordinator := NewCoordinator(ctx, logger)
	leaseParser := parser.NewParser(logger)

	syncFn := func(ctx context.Context) {
		logger.Debug("coordinator starting parse job")
		leaseParser.ParseStreamingWithHandler(ctx, leaseFile, func(lease *parser.Lease) {
			logger.Debug("found lease", zap.String("name", lease.Name), zap.String("address", lease.Address.String()))
			if err := backend.Put(ctx, lease); err != nil {
				logger.Error("failed to send lease to backend", zap.String("name", lease.Name), zap.Error(err))
			}
		})
	}

	jobStart()
	go func() {
		coordinator.Run(func() { syncFn(ctx) })
		logger.Debug("watch coordinator stopped")
		jobStop()
	}()

	jobStart()
	go func() {
		Watch(ctx, leaseFile, logger, func(event fsnotify.Event) {
			logger.Debug("received fs event", zap.String("op", event.Op.String()))
			coordinator.Signal()
		})
		logger.Debug("file watcher stopped")
		jobStop()
	}()

	return syncFn
}
