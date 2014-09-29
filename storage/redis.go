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
	log.Infof("get offline msgs (%s) (>%d)", appId, msgId)
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
		log.Infof("failed to get raw msg:", err)
		return nil
	}
	rmsg := &RawMessage{}
	if err := json.Unmarshal(ret, rmsg); err != nil {
		log.Infof("failed to decode raw msg:", err)
		return nil
	}
	return rmsg
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
		log.Infof("HSET failed, (%s)", err)
		return err
	}
	if msgId > 0 {
		if _, err := r.pool.Get().Do("HINCRBY", "db_msg_stat", fmt.Sprintf("%d", msgId), 1); err != nil {
			log.Infof("HINCRBY failed, (%s)", err)
		}
	}
	return nil
	//return &pusherror.PushError{"add failed"}
}

func (r *RedisStorage)GetApp(appId string, regId string) (*AppInfo) {
	msg, err := redis.Bytes(r.pool.Get().Do("HGET", fmt.Sprintf("db_app_%s", appId), regId))
	if err != nil {
		return nil
	}

	var app AppInfo
	if err := json.Unmarshal(msg, &app); err != nil {
		return nil
	}
	return &app
}

/*
func main() {
	r := newRedisStorage()
	log.Print(r.GetMsg("myapp1", 19))
	log.Print("\n")
	log.Print(r.GetOfflineMsgs("myapp1", 19))
}
*/
