package config

import (
	"time"

	"github.com/spf13/viper"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type Config struct {
	Etcd            clientv3.Config
	KeyPrefix       PrefixConfig
	Lease           LeaseConfig
	CleanupInterval time.Duration
	LogLevel        string
}

type PrefixConfig struct {
	Zone      string
	Heartbeat string
}

type LeaseConfig struct {
	File    string
	Timeout time.Duration
}

func SetDefaults(vp *viper.Viper) {
	vp.SetDefault("cleanupInterval", time.Minute)
	vp.SetDefault("lease.timeout", time.Minute)
	vp.SetDefault("logLevel", "info")
}
