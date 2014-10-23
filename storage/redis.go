package storage

import (
	"time"
	"fmt"
	"sort"
	"encoding/json"
	"github.com/garyburd/redigo/redis"
	"github.com/chenyf/push/conf"
	log "github.com/cihub/seelog"
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
				c, err := redis.Dial("tcp", conf.Config.Redis.Server)
				//c, err := redis.Dial("tcp", RedisServer)
				if err != nil {
					log.Infof("failed to connect Redis:", err)
					return nil, err
				}
				if _, err := c.Do("AUTH", conf.Config.Redis.Pass); err != nil {
					log.Infof("failed to auth Redis:", err)
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
func (r *RedisStorage)GetOfflineMsgs(appId string, msgId int64) []*RawMessage {
	key := appId + "_offline"
	ret, err := redis.Strings(r.pool.Get().Do("HKEYS", key))
	if err != nil {
		log.Infof("failed to get fields of offline msg:", err)
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
			log.Infof("invaild redis hash field:", err)
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

	if len(args) == 1 {
		return nil
	}

	rmsgs, err := redis.Strings(r.pool.Get().Do("HMGET", args...))
	if err != nil {
		log.Infof("failed to get offline rmsg:", err)
		return nil
	}

	var msgs []*RawMessage
	for i := range rmsgs {
		t := []byte(rmsgs[i])
		msg := &RawMessage{}
		if err := json.Unmarshal(t, msg); err != nil {
			log.Infof("failed to decode raw msg:", err)
			continue
		}
		msgs = append(msgs, msg)
	}
	return msgs
}

// 从存储后端获取指定消息
func (r *RedisStorage)GetRawMsg(appId string, msgId int64) *RawMessage {
	ret, err := redis.Bytes(r.pool.Get().Do("HGET", appId, msgId))
	if err != nil {
		log.Warnf("redis: HGET failed (%s)", err)
		return nil
	}
	rmsg := &RawMessage{}
	if err := json.Unmarshal(ret, rmsg); err != nil {
		log.Warnf("failed to decode raw msg:", err)
		return nil
	}
	return rmsg
}

func (r *RedisStorage)AddDevice(devId string) bool {
	result, err := redis.Int(r.pool.Get().Do("HSETNX", "db_device", devId, "1"))
	if err != nil {
		log.Warnf("redis: HSETNX failed (%s)", err)
		return false
	}
	if result == 1 {
		return true
	}
	return false
}

func (r *RedisStorage)RemoveDevice(devId string) {
	_, err := r.pool.Get().Do("HDEL", "db_device", devId)
	if err != nil {
		log.Warnf("redis: HDEL failed (%s)", err)
	}
}

func (r *RedisStorage)HashGetAll(db string) ([]string, error) {
	ret, err := redis.Strings(r.pool.Get().Do("HGETALL", db))
	if err != nil {
		log.Warnf("redis: HGET failed (%s)", err)
	}
	return ret, err
}

func (r *RedisStorage)HashGet(db string, key string) ([]byte, error) {
	ret, err := r.pool.Get().Do("HGET", db, key)
	if err != nil {
		log.Warnf("redis: HGET failed (%s)", err)
		return nil, err
	}
	if r != nil {
		return redis.Bytes(ret, nil)
	}
	return nil, nil
}

func (r *RedisStorage)HashSet(db string, key string, val []byte) (int, error) {
	ret, err := redis.Int(r.pool.Get().Do("HSET", db, key, val))
	if err != nil {
		log.Warnf("redis: HSET failed (%s)", err)
	}
	return ret, err
}

func (r *RedisStorage)HashSetNotExist(db string, key string, val []byte) (int, error) {
	ret, err := redis.Int(r.pool.Get().Do("HSETNX", db, key, val))
	if err != nil {
		log.Warnf("redis: HSETNX failed (%s)", err)
	}
	return ret, err
}

func (r *RedisStorage)HashDel(db string, key string) (int, error) {
	ret, err := redis.Int(r.pool.Get().Do("HDEL", db, key))
	if err != nil {
		log.Warnf("redis: HDEL failed (%s)", err)
	}
	return ret, err
}

func (r *RedisStorage)HashIncrBy(db string, key string, val int64) (int64, error) {
	ret, err := redis.Int64(r.pool.Get().Do("HINCRBY", db, key, val))
	if err != nil {
		log.Warnf("redis: HINCRBY failed, (%s)", err)
	}
	return ret, err
}

func (r *RedisStorage)SetNotExist(key string, val []byte) (int, error) {
	ret, err := redis.Int(r.pool.Get().Do("SETNX", key, val))
	if err != nil {
		log.Warnf("redis: SETNX failed (%s)", err)
	}
	return ret, err
}

func (r *RedisStorage)IncrBy(key string, val int64) (int64, error) {
	ret, err := redis.Int64(r.pool.Get().Do("INCRBY", key, val))
	if err != nil {
		log.Warnf("redis: INCRBY failed, (%s)", err)
	}
	return ret, nil
}

func (r *RedisStorage)SetAdd(key string, val string) (int, error) {
	ret, err := redis.Int(r.pool.Get().Do("SADD", key, val))
	if err != nil {
		log.Warnf("redis: SADD failed, (%s)", err)
	}
	return ret, err
}

func (r *RedisStorage)SetMove(key string, val string) (int, error) {
	ret, err := redis.Int(r.pool.Get().Do("SMOVE", key, val))
	if err != nil {
		log.Warnf("redis: SMOVE failed, (%s)", err)
	}
	return ret, err
}

func (r *RedisStorage)SetIsMember(key string, val string) (int, error) {
	ret, err := redis.Int(r.pool.Get().Do("SISMEMBER", key, val))
	if err != nil {
		log.Warnf("redis: SISMEMBER failed, (%s)", err)
	}
	return ret, err

}

func (r *RedisStorage)SetMembers(key string) ([]string, error) {
	ret, err := redis.Strings(r.pool.Get().Do("SMEMBERS", key))
	if err != nil {
		log.Warnf("redis: SMEMBERS failed, (%s)", err)
	}
	return ret, err
}

