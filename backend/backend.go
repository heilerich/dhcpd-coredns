package backend

import (
	"context"
	"fmt"
	"time"

	"github.com/heilerich/dhcpd-coredns/config"
	"inet.af/netaddr"
)

type Lease interface {
	GetName() string
	GetAddress() netaddr.IP
}

type Backend interface {
	Put(context.Context, Lease) error
	Cleanup(context.Context) error
	Close(context.Context) error
}

func LeaseID(lease Lease) string {
	addr := lease.GetAddress()
	if addr.Is6() {
		return fmt.Sprintf("%x", addr.As16())
	}
	return fmt.Sprintf("%x", addr.As4())
}

type SyncFn func(ctx context.Context)

func RunCleaner(ctx context.Context, backend Backend, syncFn SyncFn, cfg *config.Config) error {
	ticker := time.NewTicker(cfg.CleanupInterval)

	for {
		select {
		case <-ticker.C:
			syncFn(ctx)
			if err := backend.Cleanup(ctx); err != nil {
				ticker.Stop()
				return err
			}
			ticker.Reset(cfg.CleanupInterval)
		case <-ctx.Done():
			ticker.Stop()
			return nil
		}
	}
}
