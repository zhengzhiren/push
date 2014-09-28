package storage
//package main

import (
	//"time"
	"fmt"
	"log"
	"time"
	"sort"
	//"strconv"
	"encoding/json"
	"github.com/garyburd/redigo/redis"
	//"github.com/chenyf/push/error"
)

const (
	RedisServer = "10.154.156.121:6380"
	RedisPasswd = "rpasswd"
)

type RedisStorage struct {
	pool *redis.Pool
}

func newRedisStorage() *RedisStorage {
	return &RedisStorage {
		pool: &redis.Pool{
			MaxIdle: 1,
			IdleTimeout: 300 * time.Second,
			Dial: func() (redis.Conn, error) {
				c, err := redis.Dial("tcp", RedisServer)
				if err != nil {
					log.Printf("faild to connect Redis:", err)
					return nil, err
				}
				if _, err := c.Do("AUTH", RedisPasswd); err != nil {
					log.Printf("failed to auth Redis:", err)
					return nil, err

				}
				return c, err
			},
			TestOnBorrow: func(c redis.Conn, t time.Time) error {
				_, err := c.Do("PING")
				return err
			},
		},
	}
}

// 从存储后端获取 > 指定时间的所有消息
func (r *RedisStorage)GetOfflineMsgs(appId string, msgId int64) []string {
	key := appId + "_offline"
	ret, err := redis.Strings(r.pool.Get().Do("HKEYS", key))
	if err != nil {
		log.Printf("failed to get fields of offline msg:", err)
		return nil
	}

	now := time.Now().Unix()
	skeys := make(map[int64]interface{})
	var sidxs []float64

	for i := range ret {
		var (
			idx int64
			expire int64
		)
		if _, err := fmt.Sscanf(ret[i], "%v_%v", &idx, &expire); err != nil {
			log.Printf("invaild redis hash field:", err)
			continue
		}

		if idx <= msgId || expire <= now {
			continue
		} else {
			skeys[idx] = ret[i]
			sidxs = append(sidxs, float64(idx))
		}
	}

	sort.Float64Slice(sidxs).Sort()
	args := []interface{}{key}
	for k := range sidxs {
		t := int64(sidxs[k])
		args = append(args, skeys[t])
	}

	msgs, err := redis.Strings(r.pool.Get().Do("HMGET", args...))
	if err != nil {
		log.Printf("failed to get offline msg:", err)
		return nil
	}
	return msgs
}

// 从存储后端获取指定消息
func (r *RedisStorage)GetMsg(appId string, msgId int64) string {
	msg, err := redis.String(r.pool.Get().Do("HGET", appId, msgId))
	if err != nil {
		log.Printf("failed to get msg:", err)
		return ""
	}
	return msg
}

func (r *RedisStorage)UpdateApp(appId string, regId string, msgId int64) error {
	app := AppInfo{
		LastMsgId : msgId,
	}
	b, err := json.Marshal(app)
	if err != nil {
		return err
	}
	if _, err := r.pool.Get().Do("HSET", fmt.Sprintf("db_app_%s", appId), regId, b); err != nil {
		return err
	}
	return nil
	//return &pusherror.PushError{"add failed"}
}

func (r *RedisStorage)GetApp(appId string, regId string) (*AppInfo, error) {
	msg, err := redis.Bytes(r.pool.Get().Do("HGET", fmt.Sprintf("db_app_%s", appId), regId))
	if err != nil {
		return nil, err
	}

	var app AppInfo
	if err := json.Unmarshal(msg, &app); err != nil {
		return nil ,err
	}
	return &app, nil
}

/*
func main() {
	r := newRedisStorage()
	log.Print(r.GetMsg("12345678", 1))
	log.Print(r.GetOfflineMsgs("12345678", 1))
}
*/
