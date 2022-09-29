package etcd

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/heilerich/dhcpd-coredns/backend"
	"github.com/heilerich/dhcpd-coredns/config"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

type etcdBackend struct {
	client                  clientv3.Client
	dnsPrefix, configPrefix string
	leaseTimeout            time.Duration
	logger                  *zap.Logger
}

var _ backend.Backend = &etcdBackend{}

func NewEtcdBackend(cfg *config.Config, logger *zap.Logger) (*etcdBackend, error) {
	client, err := clientv3.New(cfg.Etcd)
	if err != nil {
		return nil, err
	}
	return &etcdBackend{
		client:       *client,
		dnsPrefix:    cfg.KeyPrefix.Zone,
		configPrefix: cfg.KeyPrefix.Heartbeat,
		leaseTimeout: cfg.Lease.Timeout,
		logger:       logger,
	}, nil
}

func (e *etcdBackend) buildKey(lease backend.Lease, prefix string) string {
	key := strings.TrimSuffix(prefix, "/")
	zones := strings.Split(lease.GetName(), ".")

	for i := len(zones) - 1; i >= 0; i-- {
		key = fmt.Sprintf("%v/%v", key, zones[i])
	}

	key = fmt.Sprintf("%v/%v", key, backend.LeaseID(lease))

	return key
}

func (e *etcdBackend) buildEntry(lease backend.Lease) *hostEntry {
	return &hostEntry{
		Host:  lease.GetAddress().String(),
		Group: lease.GetName(),
		TTL:   60,
	}
}

type hostEntry struct {
	Host  string `json:"host"`
	Group string `json:"group"`
	TTL   int    `json:"ttl"`
}

func (e *etcdBackend) Put(ctx context.Context, lease backend.Lease) error {
	key := e.buildKey(lease, e.dnsPrefix)

	json, err := json.Marshal(e.buildEntry(lease))
	if err != nil {
		return err
	}

	_, err = e.client.Put(ctx, key, string(json))
	if err != nil {
		return err
	}

	configKey := e.buildKey(lease, e.configPrefix)
	_, err = e.client.Put(ctx, configKey, fmt.Sprint(time.Now().UTC().Unix()))
	if err != nil {
		return err
	}

	return nil
}

func (e *etcdBackend) Cleanup(ctx context.Context) error {
	logger := e.logger.WithOptions(zap.Fields(zap.String("op", "etcd.Cleanup")))

	resp, err := e.client.Get(
		ctx, e.configPrefix,
		clientv3.WithPrefix(),
		clientv3.WithSort(clientv3.SortByValue, clientv3.SortAscend),
	)
	if err != nil {
		return err
	}

	logger.Debug("received keys", zap.Int("count", int(resp.Count)))

	for _, kv := range resp.Kvs {
		if ok := e.handleKey(ctx, string(kv.Key), string(kv.Value)); !ok {
			break
		}
	}

	return nil
}

func (e *etcdBackend) handleKey(ctx context.Context, key string, timeString string) bool {
	logger := e.logger.WithOptions(zap.Fields(zap.String("op", "etcd.remove"), zap.String("key", key)))

	timeInt, err := strconv.ParseInt(timeString, 10, 64)
	if err != nil {
		logger.Warn("deleting key with invalid heartbeat timestamp", zap.String("key", key), zap.String("value", timeString), zap.Error(err))
		if err := e.remove(ctx, key); err != nil {
			logger.Warn("failed to delete key", zap.String("key", key), zap.Error(err))
		}
		return true
	}

	created := time.Unix(timeInt, 0)
	if time.Now().UTC().Sub(created) > e.leaseTimeout {
		logger.Info("remove expired lease", zap.String("key", key))
		if err := e.remove(ctx, key); err != nil {
			logger.Warn("failed to delete key", zap.String("key", key), zap.Error(err))
		}

		key = strings.Replace(key, e.configPrefix, e.dnsPrefix, 1)
		if err := e.remove(ctx, key); err != nil {
			logger.Warn("failed to delete key", zap.String("key", key), zap.Error(err))
		}
		return true
	}

	//Reached list end
	return false
}

func (e *etcdBackend) remove(ctx context.Context, key string) error {
	logger := e.logger.WithOptions(zap.Fields(zap.String("op", "etcd.remove"), zap.String("key", key)))

	resp, err := e.client.Delete(ctx, key)
	if err != nil {
		return err
	}

	if resp.Deleted != 1 {
		logger.Warn("deletion removed wrong number of keys", zap.Int("expected", 1), zap.Int("actual", int(resp.Deleted)))
	}

	return nil
}

func (e *etcdBackend) Close(ctx context.Context) error {
	return e.client.Close()
}
