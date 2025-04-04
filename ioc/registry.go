package ioc

import (
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/crazyfrankie/thumbs/config"
)

func InitRegistry() *clientv3.Client {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{config.GetConf().ETCD.Addr},
		DialTimeout: time.Second * 2,
	})
	if err != nil {
		panic(err)
	}

	return client
}
