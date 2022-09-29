package parser_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/heilerich/dhcpd-coredns/parser"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
	"inet.af/netaddr"
)

var expectation = []*parser.Lease{
	{Name: "k8s-master-worker-64bf8b486f-qqd2c", Address: netaddr.MustParseIP("10.90.32.80")},
	{Name: "k8s-master-worker-64bf8b486f-nhg29", Address: netaddr.MustParseIP("10.90.32.90")},
	{Name: "k8s-master-worker-64bf8b486f-frgks", Address: netaddr.MustParseIP("10.90.32.94")},
	{Name: "tzdim-dachstein", Address: netaddr.MustParseIP("10.90.36.105")},
	{Name: "unz-hans-lab-default-6898b454f4-h4xkk", Address: netaddr.MustParseIP("10.90.36.86")},
	{Name: "tzdim-dev-default-647ff57c9b-8vf78", Address: netaddr.MustParseIP("10.90.36.113")},
	{Name: "tzdim-dev-default-647ff57c9b-tq2gl", Address: netaddr.MustParseIP("10.90.36.117")},
	{Name: "tzdim-dev-default-647ff57c9b-vdcd5", Address: netaddr.MustParseIP("10.90.36.109")},
	{Name: "tzdim-dev-default-647ff57c9b-64nnm", Address: netaddr.MustParseIP("10.90.36.119")},
	{Name: "unz-hans-lab-default-6898b454f4-nwkbp", Address: netaddr.MustParseIP("10.90.36.84")},
	{Name: "k8s-master-worker-79f8cf78db-cbdtz", Address: netaddr.MustParseIP("10.90.36.160")},
	{Name: "k8s-master-worker-79f8cf78db-jt462", Address: netaddr.MustParseIP("10.90.36.152")},
	{Name: "tzdim-dev-default-647ff57c9b-kvnr2", Address: netaddr.MustParseIP("10.90.36.111")},
	{Name: "diz-dev-worker-7gcpvv-8555586f6d-5vvf7", Address: netaddr.MustParseIP("10.90.36.185")},
	{Name: "tzdim-marmolata", Address: netaddr.MustParseIP("10.90.36.68")},
	{Name: "diz-dev-worker-7gcpvv-8555586f6d-7sbsw", Address: netaddr.MustParseIP("10.90.36.187")},
	{Name: "diz-dev-worker-7gcpvv-8555586f6d-zdms9", Address: netaddr.MustParseIP("10.90.36.189")},
}

func TestParsing(t *testing.T) {
	logger := zaptest.NewLogger(t)
	testParser := parser.NewParser(logger)
	leases, err := testParser.ParseFile("testdata/leases.example")
	if err != nil {
		t.Errorf("Parsing failed with error: %v", err)
	}

	assert.Len(t, leases, len(expectation), "expect lease list length to match")
	assert.ElementsMatch(t, leases, expectation, "expect parsed leases to match")
}

func TestStreamingParse(t *testing.T) {
	logger := zaptest.NewLogger(t)
	testParser := parser.NewParser(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	ch := testParser.ParseStreaming(ctx, "testdata/leases.example")

	leases := []*parser.Lease{}
	for {
		select {
		case lease, ok := <-ch:
			if !ok {
				ch = nil
				break
			}
			leases = append(leases, lease)
		case <-ctx.Done():
			ch = nil
		}

		if ch == nil {
			break
		}
	}

	assert.NoError(t, ctx.Err(), "expect context to not time out")
	assert.Len(t, leases, len(expectation), "expect lease list length to match")
	assert.ElementsMatch(t, leases, expectation, "expect parsed leases to match")
}

func TestStreamingParseWithHandler(t *testing.T) {
	logger := zaptest.NewLogger(t)
	testParser := parser.NewParser(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	wg := &sync.WaitGroup{}
	wg.Add(len(expectation))
	testParser.ParseStreamingWithHandler(ctx, "testdata/leases.example", func(lease *parser.Lease) {
		wg.Done()
	})

	done := make(chan struct{})
	go func() {
		wg.Wait()
		done <- struct{}{}
	}()

	select {
	case <-done:
	case <-ctx.Done():
		t.Error("context timed out")
	}
}
