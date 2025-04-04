package config

import (
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/joho/godotenv"
	"github.com/kr/pretty"
	"github.com/spf13/viper"
)

var (
	once sync.Once
	conf *Config
)

type Config struct {
	Env    string
	Server Server `yaml:"server"`
	TiDB   TiDB   `yaml:"tidb"`
	Redis  Redis  `yaml:"redis"`
	ETCD   ETCD   `yaml:"etcd"`
}

type Server struct {
	Port string `yaml:"port"`
	TTL  int64  `yaml:"ttl"`
}

type TiDB struct {
	DSN string `yaml:"dsn"`
}

type Redis struct {
	Addr string `yaml:"addr"`
}

type ETCD struct {
	Addr string `yaml:"addr"`
}

func GetConf() *Config {
	once.Do(func() {
		initConf()
	})
	return conf
}

func initConf() {
	prefix := "config"
	err := godotenv.Load(filepath.Join(prefix, ".env"))
	if err != nil {
		panic(err)
	}

	path := filepath.Join(prefix, filepath.Join(GetEnv(), "conf.yaml"))
	viper.SetConfigFile(path)
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Failed to read config file: %v", err)
	}

	conf = new(Config)
	if err := viper.Unmarshal(&conf); err != nil {
		log.Fatalf("Failed to unmarshal config: %v", err)
	}

	conf.Env = GetEnv()
	// 打印配置，方便调试
	pretty.Printf("%+v\n", conf)
}

func GetEnv() string {
	e := os.Getenv("GO_ENV")
	if len(e) == 0 {
		return "test"
	}
	return e
}
