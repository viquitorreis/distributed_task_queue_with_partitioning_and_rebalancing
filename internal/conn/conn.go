package conn

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	etcd "go.etcd.io/etcd/client/v3"
)

type DBConn struct {
	redis *redis.Client
	etcd  *etcd.Client

	mu sync.RWMutex
}

type IConn interface {
	GetRedis() *redis.Client
	GetEtcd() *etcd.Client
	Close()
}

func NewConn() IConn {
	return &DBConn{
		redis: getRedis(),
		etcd:  getEtcd(),
	}
}

func (c *DBConn) GetRedis() *redis.Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.redis
}

func (c *DBConn) GetEtcd() *etcd.Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.etcd
}

func (c *DBConn) Close() {
	fmt.Println("closing conns...")
	c.mu.Lock()
	defer c.mu.Unlock()
	c.redis.Close()
	c.etcd.Close()
}

func getRedis() *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6543",
		Password: "",
		DB:       0,
		Protocol: 2,
	})

	if rdb == nil {
		log.Fatal("redis was not found. Run 'make setup'")
	}

	return rdb
}

func getEtcd() *etcd.Client {
	cli, err := etcd.New(etcd.Config{
		Endpoints:   []string{"localhost:2379", "localhost:22379", "localhost:32379"},
		DialTimeout: time.Second * 3,
	})
	if err != nil {
		log.Fatalf("couldnt get etcd cli: %s\n", err)
	}

	if cli == nil {
		log.Fatal("etcd was not found. Run 'make setup'")
	}

	return cli
}
