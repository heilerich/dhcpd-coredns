package e2e_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/heilerich/dhcpd-coredns/backend/etcd"
	"github.com/heilerich/dhcpd-coredns/config"
	"github.com/heilerich/dhcpd-coredns/watcher"
	"github.com/stretchr/testify/assert"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap/zaptest"
)

func TestEtcdWithLeaseFile(t *testing.T) {
	logger := zaptest.NewLogger(t)

	leaseFile, err := os.CreateTemp("", "test-leases-")
	if err != nil {
		t.Fatalf("failed to create temporary file: %v", err)
	}
	defer leaseFile.Close()
	defer os.Remove(leaseFile.Name())

	zoneSuffix := randomSuffix(5)
	zonePrefix := fmt.Sprintf("/skydns/test/%v/", zoneSuffix)
	heartBeatPrefix := fmt.Sprintf("/dhcpd/%v/", zoneSuffix)

	cfg := &config.Config{
		Etcd: clientv3.Config{
			Endpoints: []string{"http://etcd:2379"},
			Username:  "test-user",
			Password:  "test-pass",
		},
		KeyPrefix: config.PrefixConfig{
			Zone:      zonePrefix,
			Heartbeat: heartBeatPrefix,
		},
		Lease: config.LeaseConfig{
			File:    leaseFile.Name(),
			Timeout: 0,
		},
	}

	backend, err := etcd.NewEtcdBackend(cfg, logger)
	if err != nil {
		t.Fatalf("failed to initialize backend: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	watcher.CoordinateWatcher(ctx, leaseFile.Name(), backend, logger, func() {}, func() {})

	// give fs watcher time to start
	time.Sleep(10 * time.Millisecond)

	writeFixtures := writeFixturesFactory(t, leaseFile)
	resolver := testResolver(t, ctx, zoneSuffix)

	fixtures := createFixtures(t,
		"test1", "1.1.1.1",
		"test2", "2001:db8::2",
		"test3", "1.1.1.3",
		"test3", "1.1.2.3",
	)

	writeFixtures(fixtures...)

	resolver.AssertLeaseFixture(fixtures)

	err = backend.Cleanup(ctx)
	assert.NoError(t, err, "no error cleaning up")

	time.Sleep(20 * time.Millisecond)
	ips := resolver.LookupName("test1")
	assert.Empty(t, ips, "no such host after cleanup")

	cancel()
	time.Sleep(10 * time.Millisecond)
	leaseFile.Close()
	os.Remove(leaseFile.Name())
}
