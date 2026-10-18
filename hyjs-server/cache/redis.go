package cache

import (
	"context"
	"hyjs-server/config"
	"hyjs-server/elog"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var redisClient *redis.Client

func Init() error {
	client := redis.NewClient(
		&redis.Options{
			Addr:     config.AppConfigIns.Redis.Host,
			Password: config.AppConfigIns.Redis.Password,
			DB:       config.AppConfigIns.Redis.Db,
		})

	cmd := client.Ping(context.TODO())
	if cmd.Err() != nil {
		elog.Error("redis init error", zap.Error(cmd.Err()))
		return cmd.Err()
	}

	redisClient = client
	return nil
}

func GetClient() *redis.Client {
	return redisClient
}
