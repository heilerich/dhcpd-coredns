package watcher_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/heilerich/dhcpd-coredns/watcher"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

func TestCoordinator(t *testing.T) {
	logger := zaptest.NewLogger(t)

	callMu := sync.Mutex{}
	calls := make([]bool, 0)
	callEnter := make([]chan struct{}, 0)
	callSignal := make([]chan struct{}, 0)

	coordinatorContext, cancelCoordinator := context.WithCancel(context.Background())
	coordinator := watcher.NewCoordinator(coordinatorContext, logger)

	signal := func() {
		callMu.Lock()
		defer callMu.Unlock()
		calls = append(calls, false)
		callSignal = append(callSignal, make(chan struct{}))
		callEnter = append(callEnter, make(chan struct{}))
		coordinator.Signal()
	}

	countCalls := func() int {
		callMu.Lock()
		defer callMu.Unlock()

		callCount := 0
		for _, called := range calls {
			if called {
				callCount += 1
			}
		}

		return callCount
	}

	stop := make(chan struct{})

	go func() {
		coordinator.Run(func() {
			callMu.Lock()
			callIndex := 0
			for i, called := range calls {
				if !called {
					callEnter[i] <- struct{}{}
					calls[i] = true
					callIndex = i
					break
				}
			}
			callMu.Unlock()
			<-callSignal[callIndex]
		})
		stop <- struct{}{}
	}()

	assert.Equal(t, 0, countCalls(), "before signal")

	signal()
	<-callEnter[0]
	assert.Equal(t, 1, countCalls(), "after one signal")

	signal()
	time.Sleep(10 * time.Millisecond)
	assert.Equal(t, 1, countCalls(), "after two signals")

	signal()
	time.Sleep(10 * time.Millisecond)
	assert.Equal(t, 1, countCalls(), "after three signals")

	signal()
	time.Sleep(10 * time.Millisecond)
	assert.Equal(t, 1, countCalls(), "after four signals")

	callSignal[0] <- struct{}{}
	<-callEnter[1]
	assert.Equal(t, 2, countCalls(), "after first stop")

	signal()

	callSignal[1] <- struct{}{}
	<-callEnter[2]
	assert.Equal(t, 3, countCalls(), "after second stop")

	callSignal[2] <- struct{}{}
	time.Sleep(10 * time.Millisecond)
	assert.Equal(t, 3, countCalls(), "after third stop")

	cancelCoordinator()
	<-stop
	assert.Equal(t, 3, countCalls(), "after stop end")
}
