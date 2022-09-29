package util

import (
	"context"
	"sync"
	"time"
)

type TimeoutGroup struct {
	sync.WaitGroup
}

func (g *TimeoutGroup) WaitWithTimeout(ctx context.Context, timeout time.Duration) error {
	done := make(chan struct{})

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	go func() {
		g.Wait()
		done <- struct{}{}
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
