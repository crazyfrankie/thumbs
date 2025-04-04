package ioc

import (
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"github.com/crazyfrankie/thumbs/config"
)

func InitDB() *gorm.DB {
	dsn := fmt.Sprintf(config.GetConf().TiDB.DSN,
		os.Getenv("TIDB_USER"),
		os.Getenv("TIDB_PASSWORD"),
		os.Getenv("TIDB_HOST"),
		os.Getenv("TIDB_PORT"),
		os.Getenv("TIDB_DB"))
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
	})
	if err != nil {
		panic(err)
	}

	return db
}

func InitRedis() redis.Cmdable {
	client := redis.NewClient(&redis.Options{
		Network:      "tcp",
		Addr:         config.GetConf().Redis.Addr,
		DialTimeout:  time.Second,
		ReadTimeout:  time.Second * 3,
		WriteTimeout: time.Second * 5,
		PoolSize:     20,
	})

	return client
}
