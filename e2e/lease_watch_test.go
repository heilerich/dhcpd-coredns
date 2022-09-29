package e2e_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/heilerich/dhcpd-coredns/backend"
	"github.com/heilerich/dhcpd-coredns/parser"
	"github.com/heilerich/dhcpd-coredns/watcher"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"inet.af/netaddr"
)

type backendMock struct {
	mock.Mock
}

func (b *backendMock) Put(lease backend.Lease) error {
	args := b.Called(lease)
	return args.Error(0)
}

func (b *backendMock) WaitForExpectations(ctx context.Context) {
	for {
		if ctx.Err() != nil || b.AssertExpectations(&testing.T{}) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (b *backendMock) ExpectLeases(leases ...*parser.Lease) {
	for _, lease := range leases {
		b.On("Put", lease).Return(nil)
	}
}

func TestWatchingLeaseFileMock(t *testing.T) {
	logger := zaptest.NewLogger(t)

	leaseFile, err := os.CreateTemp("", "test-leases-")
	if err != nil {
		t.Fatalf("failed to create temporary file: %v", err)
	}
	defer leaseFile.Close()
	defer os.Remove(leaseFile.Name())

	backend := &backendMock{}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	coordinator := watcher.NewCoordinator(ctx, logger)
	leaseParser := parser.NewParser(logger)

	go coordinator.Run(func() {
		logger.Debug("coordinator starting parse job")
		leaseParser.ParseStreamingWithHandler(ctx, leaseFile.Name(), func(lease *parser.Lease) {
			logger.Debug("append lease", zap.String("name", lease.Name), zap.String("address", lease.Address.String()))
			backend.Put(lease)
		})
	})

	go watcher.Watch(ctx, leaseFile.Name(), logger, func(event fsnotify.Event) {
		logger.Debug("received fs event", zap.String("op", event.Op.String()))
		coordinator.Signal()
	})

	time.Sleep(10 * time.Millisecond)

	backend.AssertExpectations(t)

	writeFixtures := writeFixturesFactory(t, leaseFile)

	fixtures := createFixtures(t,
		"test1", "1.1.1.1",
		"test2", "2001:db8::2",
		"test3", "1.1.1.3",
	)

	backend.ExpectLeases(fixtures...)
	writeFixtures(fixtures...)
	backend.WaitForExpectations(ctx)

	backend.AssertExpectations(t)

	fixtures = createFixtures(t,
		"2-test1", "2.1.1.1",
		"2-test2", "2001:db8:2::2",
		"2-test3", "2.1.1.3",
	)

	backend.ExpectLeases(fixtures...)

	leaseFile.Seek(0, 0)
	writeFixtures(fixtures...)

	backend.WaitForExpectations(ctx)

	appendedLease := &parser.Lease{Name: "3-test1", Address: netaddr.MustParseIP("3.1.1.1")}
	backend.ExpectLeases(appendedLease)
	writeFixtures(appendedLease)

	backend.WaitForExpectations(ctx)

	backend.AssertExpectations(t)

	cancel()
	time.Sleep(10 * time.Millisecond)
	leaseFile.Close()
	os.Remove(leaseFile.Name())
}
