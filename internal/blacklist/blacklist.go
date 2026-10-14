package blacklist

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

const KeyPrefix = "ebike_battery.jwt_blacklist."

const (
	dialTimeout = 5 * time.Second
	readTimeout = 5 * time.Second
)

type Config struct {
	Host     string
	Port     string
	Password string
}

func ConfigFromEnv() Config {
	cfg := Config{Host: os.Getenv("REDIS_HOST"), Port: os.Getenv("REDIS_PORT"), Password: os.Getenv("REDIS_PASSWORD")}
	if cfg.Host == "" {
		cfg.Host = "localhost"
	}
	if cfg.Port == "" {
		cfg.Port = "6379"
	}
	return cfg
}

type Blacklist struct {
	client *redis.Client
}

func New(ctx context.Context, cfg Config) (*Blacklist, error) {
	client := redis.NewClient(&redis.Options{
		Addr:        cfg.Host + ":" + cfg.Port,
		Password:    cfg.Password,
		DB:          0,
		DialTimeout: dialTimeout,
		ReadTimeout: readTimeout,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis не отвечает: %w", err)
	}
	return &Blacklist{client: client}, nil
}

func (b *Blacklist) Close() error {
	return b.client.Close()
}

func (b *Blacklist) Add(ctx context.Context, token, riderLogin string, ttl time.Duration) error {
	return b.client.Set(ctx, KeyPrefix+token, riderLogin, ttl).Err()
}

func (b *Blacklist) Contains(ctx context.Context, token string) (bool, error) {
	err := b.client.Get(ctx, KeyPrefix+token).Err()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
