package storage

import (
	"encoding/json"
	"fmt"
	"github.com/chenyf/push/conf"
	log "github.com/cihub/seelog"
	"github.com/garyburd/redigo/redis"
	"sort"
	"time"
)

const (
	cometNodesKey = "comet_nodes"
)

type RedisStorage struct {
	pool  *redis.Pool
	retry int
}

func NewRedisStorage(config *conf.ConfigStruct) *RedisStorage {
	return newRedisStorage(
		config.Redis.Server,
		config.Redis.Pass,
		config.Redis.MaxActive,
		config.Redis.MaxIdle,
		config.Redis.IdleTimeout,
		config.Redis.Retry,
		config.Redis.ConnTimeout,
		config.Redis.ReadTimeout,
		config.Redis.WriteTimeout)
}

func newRedisStorage(server string, pass string, maxActive, maxIdle, idleTimeout, retry, cto, rto, wto int) *RedisStorage {
	return &RedisStorage{
		pool: &redis.Pool{
			MaxActive:   maxActive,
			MaxIdle:     maxIdle,
			IdleTimeout: time.Duration(idleTimeout) * time.Second,
			Dial: func() (redis.Conn, error) {
				//c, err := redis.Dial("tcp", server)
				c, err := redis.DialTimeout("tcp", server, cto, rto, wto)
				if err != nil {
					log.Warnf("failed to connect Redis (%s), (%s)", server, err)
					return nil, err
				}
				if pass != "" {
					if _, err := c.Do("AUTH", pass); err != nil {
						log.Warnf("failed to auth Redis (%s), (%s)", server, err)
						return nil, err
					}
				}
				//log.Debugf("connected with Redis (%s)", server)
				return c, err
			},
			TestOnBorrow: func(c redis.Conn, t time.Time) error {
				_, err := c.Do("PING")
				return err
			},
		},
		retry: retry,
	}
}

func (r *RedisStorage) Do(commandName string, args ...interface{}) (interface{}, error) {
	var conn redis.Conn
	i := r.retry
	for ; i > 0; i-- {
		conn = r.pool.Get()
		err := conn.Err()
		if err == nil {
			break
		} else {
			//log.Warnf("failed to get conn from pool (%s)", err)
		}
		time.Sleep(2 * time.Second)
	}
	if i == 0 || conn == nil {
		return nil, fmt.Errorf("failed to find a useful redis conn")
	} else {
		ret, err := conn.Do(commandName, args...)
		conn.Close()
		return ret, err
	}
}

// 从存储后端获取 > 指定时间的所有消息
func (r *RedisStorage) GetOfflineMsgs(appId string, regId string, regTime int64, msgId int64) []*RawMessage {
	key := "db_offline_msg_" + appId
	ret, err := redis.Strings(r.Do("HKEYS", key))
	if err != nil {
		log.Warnf("failed to get fields of offline msg:", err)
		return nil
	}

	now := time.Now().Unix()
	skeys := make(map[int64]interface{})
	var sidxs []float64

	for i := range ret {
		var (
			idx    int64
			ctime  int64
			expire int64
		)
		if _, err := fmt.Sscanf(ret[i], "%v_%v_%v", &idx, &ctime, &expire); err != nil {
			log.Warnf("invaild redis hash field:", err)
			continue
		}
		if ctime <= regTime { // the message created before app register
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

	rmsgs, err := redis.Strings(r.Do("HMGET", args...))
	if err != nil {
		log.Warnf("failed to get offline rmsg:", err)
		return nil
	}

	var msgs []*RawMessage
	for i := range rmsgs {
		t := []byte(rmsgs[i])
		msg := &RawMessage{}
		if err := json.Unmarshal(t, msg); err != nil {
			log.Warnf("failed to decode raw msg:", err)
			continue
		}
		msgs = append(msgs, msg)
	}
	return msgs
}

// 从存储后端获取指定消息
func (r *RedisStorage) GetRawMsg(appId string, msgId int64) *RawMessage {
	key := "db_msg_" + appId
	ret, err := redis.Bytes(r.Do("HGET", key, msgId))
	if err != nil {
		log.Warnf("redis: HGET failed (%s)", err)
		return nil
	}
	log.Debugf("RAW message: (%s)", string(ret))
	rmsg := &RawMessage{}
	if err := json.Unmarshal(ret, rmsg); err != nil {
		log.Warnf("failed to decode raw msg:", err)
		return nil
	}
	return rmsg
}

func (r *RedisStorage) AddComet(serverName string) error {
	_, err := r.Do("SADD", cometNodesKey, serverName)
	return err
}

func (r *RedisStorage) RemoveComet(serverName string) error {
	_, err := r.Do("SREM", cometNodesKey, serverName)
	return err
}

func (r *RedisStorage) AddDevice(serverName, devId string) error {
	_, err := redis.Int(r.Do("HSET", "comet:"+serverName, devId, nil))
	return err
}

func (r *RedisStorage) RemoveDevice(serverName, devId string) error {
	_, err := r.Do("HDEL", "comet:"+serverName, devId)
	return err
}

func (r *RedisStorage) GetServerNames() ([]string, error) {
	return redis.Strings(r.Do("SMEMBERS", cometNodesKey))
}

func (r *RedisStorage) GetDeviceIds(serverName string) ([]string, error) {
	return redis.Strings(r.Do("HKEYS", "comet:"+serverName))
}

func (r *RedisStorage) CheckDevice(devId string) (string, error) {
	names, err := r.GetServerNames()
	if err != nil {
		return "", err
	}
	for _, name := range names {
		key := "comet:" + name
		exist, err := redis.Bool(r.Do("HEXISTS", key, devId))
		if err != nil {
			return "", err
		}
		if exist {
			return name, nil
		}
	}
	return "", nil
}

func (r *RedisStorage) RefreshDevices(serverName string, timeout int) error {
	_, err := redis.Int(r.Do("EXPIRE", "comet:"+serverName, timeout))
	if err != nil {
		log.Warnf("redis: EXPIRE failed, (%s)", err)
	}
	return err
}

func (r *RedisStorage) InitDevices(serverName string) error {
	_, err := redis.Int(r.Do("DEL", "comet:"+serverName))
	if err != nil {
		log.Warnf("redis: DEL failed, (%s)", err)
	}
	return err
}

func (r *RedisStorage) initStatsService(service string) error {
	_, err := redis.Int(r.Do("SADD", "stats_service", service))
	if err != nil {
		log.Warnf("redis: SADD failed, (%s)", err)
	}
	return err
}

func (r *RedisStorage) GetStatsServices() ([]string, error) {
	v, err := redis.Strings(r.Do("SMEMBERS", "stats_service"))
	if err != nil {
		log.Warnf("redis: SMEMBERS failed, (%s)", err)
		return nil, err
	}
	return v, nil
}

func (r *RedisStorage) IncCmd(service string) error {
	if err := r.initStatsService(service); err != nil {
		return err
	}
	_, err := redis.Int(r.Do("INCR", "stats_cmd_"+service))
	if err != nil {
		log.Warnf("redis: INCR failed, (%s)", err)
	}
	return err
}

func (r *RedisStorage) GetStatsCmd(service string) (int, error) {
	v, err := redis.Int(r.Do("GET", "stats_cmd_"+service))
	if err != nil {
		return 0, err
	}
	return v, nil
}

func (r *RedisStorage) IncCmdSuccess(service string) error {
	if err := r.initStatsService(service); err != nil {
		return err
	}
	_, err := redis.Int(r.Do("INCR", "stats_cmd_success_"+service))
	if err != nil {
		log.Warnf("redis: INCR failed, (%s)", err)
	}
	return err
}

func (r *RedisStorage) GetStatsCmdSuccess(service string) (int, error) {
	v, err := redis.Int(r.Do("GET", "stats_cmd_success_"+service))
	if err != nil {
		return 0, err
	}
	return v, nil
}

func (r *RedisStorage) IncCmdTimeout(service string) error {
	if err := r.initStatsService(service); err != nil {
		return err
	}
	_, err := redis.Int(r.Do("INCR", "stats_cmd_timeout_"+service))
	if err != nil {
		log.Warnf("redis: INCR failed, (%s)", err)
	}
	return err
}

func (r *RedisStorage) GetStatsCmdTimeout(service string) (int, error) {
	v, err := redis.Int(r.Do("GET", "stats_cmd_timeout_"+service))
	if err != nil {
		return 0, err
	}
	return v, nil
}

func (r *RedisStorage) IncCmdOffline(service string) error {
	if err := r.initStatsService(service); err != nil {
		return err
	}
	_, err := redis.Int(r.Do("INCR", "stats_cmd_offline_"+service))
	if err != nil {
		log.Warnf("redis: INCR failed, (%s)", err)
	}
	return err
}

func (r *RedisStorage) GetStatsCmdOffline(service string) (int, error) {
	v, err := redis.Int(r.Do("GET", "stats_cmd_offline_"+service))
	if err != nil {
		return 0, err
	}
	return v, nil
}

func (r *RedisStorage) IncCmdInvalidService(service string) error {
	if err := r.initStatsService(service); err != nil {
		return err
	}
	_, err := redis.Int(r.Do("INCR", "stats_cmd_invalid_service_"+service))
	if err != nil {
		log.Warnf("redis: INCR failed, (%s)", err)
	}
	return err
}

func (r *RedisStorage) GetStatsCmdInvalidService(service string) (int, error) {
	v, err := redis.Int(r.Do("GET", "stats_cmd_invalid_service_"+service))
	if err != nil {
		return 0, err
	}
	return v, nil
}

func (r *RedisStorage) IncCmdOtherError(service string) error {
	if err := r.initStatsService(service); err != nil {
		return err
	}
	_, err := redis.Int(r.Do("INCR", "stats_cmd_other_error_"+service))
	if err != nil {
		log.Warnf("redis: INCR failed, (%s)", err)
	}
	return err
}

func (r *RedisStorage) GetStatsCmdOtherError(service string) (int, error) {
	v, err := redis.Int(r.Do("GET", "stats_cmd_other_error_"+service))
	if err != nil {
		return 0, err
	}
	return v, nil
}

func (r *RedisStorage) IncQueryOnlineDevices() error {
	_, err := redis.Int(r.Do("INCR", "stats_query_online_devices"))
	if err != nil {
		log.Warnf("redis: INCR failed, (%s)", err)
	}
	return err
}

func (r *RedisStorage) GetStatsQueryOnlineDevices() (int, error) {
	v, err := redis.Int(r.Do("GET", "stats_query_online_devices"))
	if err != nil {
		return 0, err
	}
	return v, nil
}

func (r *RedisStorage) IncQueryDeviceInfo() error {
	_, err := redis.Int(r.Do("INCR", "stats_query_device_info"))
	if err != nil {
		log.Warnf("redis: INCR failed, (%s)", err)
	}
	return err
}

func (r *RedisStorage) GetStatsQueryDeviceInfo() (int, error) {
	v, err := redis.Int(r.Do("GET", "stats_query_device_info"))
	if err != nil {
		return 0, err
	}
	return v, nil
}

func (r *RedisStorage) IncReplyTooLate() error {
	_, err := redis.Int(r.Do("INCR", "stats_reply_too_late"))
	if err != nil {
		log.Warnf("redis: INCR failed, (%s)", err)
	}
	return err
}

func (r *RedisStorage) GetStatsReplyTooLate() (int, error) {
	v, err := redis.Int(r.Do("GET", "stats_reply_too_late"))
	if err != nil {
		return 0, err
	}
	return v, nil
}

func (r *RedisStorage) ClearStats() error {
	services, err := r.GetStatsServices()
	for _, service := range services {
		_, err := redis.Int(r.Do("DEL",
			"stats_cmd_"+service,
			"stats_cmd_success_"+service,
			"stats_cmd_timeout_"+service,
			"stats_cmd_offline_"+service,
			"stats_cmd_invalid_service_"+service,
			"stats_cmd_other_error_"+service,
		))
		if err != nil {
			return err
		}
	}
	_, err = redis.Int(r.Do("DEL",
		"stats_query_online_devices",
		"stats_query_device_info",
		"stats_reply_too_late",
		"stats_service"))
	return err
}

func (r *RedisStorage) MsgStatsSend(msgId int64) error {
	_, err := r.Do("HINCRBY", fmt.Sprintf("db_msg_stat:%d", msgId), "send", 1)
	return err
}

func (r *RedisStorage) MsgStatsReceived(msgId int64) error {
	_, err := r.Do("HINCRBY", fmt.Sprintf("db_msg_stat:%d", msgId), "received", 1)
	return err
}

func (r *RedisStorage) MsgStatsClick(msgId int64) error {
	_, err := r.Do("HINCRBY", fmt.Sprintf("db_msg_stat:%d", msgId), "click", 1)
	return err
}

func (r *RedisStorage) GetMsgStats(msgId int64) (int, int, int, error) {
	key := fmt.Sprintf("db_msg_stat:%d", msgId)
	ret, err := redis.Values(r.Do("HMGET", key, "send", "received", "click"))
	if err != nil {
		log.Warnf("redis: HGET failed (%s)", err)
		return 0, 0, 0, err
	}
	if ret == nil {
		return 0, 0, 0, nil
	}
	send := 0
	if ret[0] != nil {
		send, _ = redis.Int(ret[0], nil)
	}
	received := 0
	if ret[1] != nil {
		received, _ = redis.Int(ret[1], nil)
	}
	click := 0
	if ret[2] != nil {
		click, _ = redis.Int(ret[2], nil)
	}
	return send, received, click, nil
}

func dateKey(date time.Time) string {
	return date.Format("20060102")
}

func (r *RedisStorage) AppStatsPushApi(appId string) error {
	key := fmt.Sprintf("stats_app_pushapi:%s", dateKey(time.Now()))
	_, err := r.Do("ZINCRBY", key, 1, appId)
	return err
}

func (r *RedisStorage) AppStatsSend(appId string) error {
	key := fmt.Sprintf("stats_app_send:%s", dateKey(time.Now()))
	_, err := r.Do("ZINCRBY", key, 1, appId)
	return err
}

func (r *RedisStorage) AppStatsReceived(appId string) error {
	key := fmt.Sprintf("stats_app_received:%s", dateKey(time.Now()))
	_, err := r.Do("ZINCRBY", key, 1, appId)
	return err
}

func (r *RedisStorage) AppStatsClick(appId string) error {
	key := fmt.Sprintf("stats_app_click:%s", dateKey(time.Now()))
	_, err := r.Do("ZINCRBY", key, 1, appId)
	return err
}

func (r *RedisStorage) GetAppStats(appId string, start time.Time, end time.Time) ([]*AppStats, error) {
	appStats := []*AppStats{}
	for ; !start.After(end); start = start.Add(24 * time.Hour) {
		date := dateKey(start)

		key := fmt.Sprintf("stats_app_pushapi:%s", date)
		pushapi, err := redis.Int(r.Do("ZSCORE", key, appId))
		if err != nil && err != redis.ErrNil {
			return nil, err
		}

		key = fmt.Sprintf("stats_app_send:%s", date)
		send, err := redis.Int(r.Do("ZSCORE", key, appId))
		if err != nil && err != redis.ErrNil {
			return nil, err
		}

		key = fmt.Sprintf("stats_app_received:%s", date)
		received, err := redis.Int(r.Do("ZSCORE", key, appId))
		if err != nil && err != redis.ErrNil {
			return nil, err
		}

		key = fmt.Sprintf("stats_app_click:%s", date)
		click, err := redis.Int(r.Do("ZSCORE", key, appId))
		if err != nil && err != redis.ErrNil {
			return nil, err
		}

		stats := AppStats{
			Date:     date,
			PushApi:  pushapi,
			Send:     send,
			Received: received,
			Click:    click,
		}
		appStats = append(appStats, &stats)
	}
	return appStats, nil
}

func (r *RedisStorage) GetSysStats(start time.Time, end time.Time) ([]*AppStats, error) {
	f := func(key string) (int, error) {
		appIds, err := redis.Strings(r.Do("ZRANGE", key, 0, -1))
		if err != nil {
			return 0, err
		}
		total := 0
		for _, appId := range appIds {
			count, err := redis.Int(r.Do("ZSCORE", key, appId))
			if err != nil && err != redis.ErrNil {
				return 0, err
			}
			total += count
		}
		return total, nil
	}

	appStats := []*AppStats{}
	for ; !start.After(end); start = start.Add(24 * time.Hour) {
		date := dateKey(start)

		key := fmt.Sprintf("stats_app_pushapi:%s", date)
		pushapi, err := f(key)
		if err != nil {
			return nil, err
		}

		key = fmt.Sprintf("stats_app_send:%s", date)
		send, err := f(key)
		if err != nil {
			return nil, err
		}

		key = fmt.Sprintf("stats_app_received:%s", date)
		received, err := f(key)
		if err != nil {
			return nil, err
		}

		key = fmt.Sprintf("stats_app_click:%s", date)
		click, err := f(key)
		if err != nil {
			return nil, err
		}

		stats := AppStats{
			Date:     date,
			PushApi:  pushapi,
			Send:     send,
			Received: received,
			Click:    click,
		}
		appStats = append(appStats, &stats)
	}
	return appStats, nil
}

func (r *RedisStorage) HashGetAll(db string) ([]string, error) {
	ret, err := r.Do("HGETALL", db)
	if err != nil {
		log.Warnf("redis: HGET failed (%s)", err)
		return nil, err
	}
	if ret != nil {
		ret, err := redis.Strings(ret, nil)
		if err != nil {
			log.Warnf("redis: convert to strings failed (%s)", err)
		}
		return ret, err
	}
	return nil, nil
}

func (r *RedisStorage) HashGet(db string, key string) ([]byte, error) {
	ret, err := r.Do("HGET", db, key)
	if err != nil {
		log.Warnf("redis: HGET failed (%s)", err)
		return nil, err
	}
	if ret != nil {
		ret, err := redis.Bytes(ret, nil)
		if err != nil {
			log.Errorf("redis: convert to bytes failed (%s)", err)
		}
		return ret, err
	}
	return nil, nil
}

func (r *RedisStorage) HashSet(db string, key string, val []byte) (int, error) {
	ret, err := redis.Int(r.Do("HSET", db, key, val))
	if err != nil {
		log.Warnf("redis: HSET failed (%s)", err)
	}
	return ret, err
}

func (r *RedisStorage) HashExists(db string, key string) (int, error) {
	ret, err := redis.Int(r.Do("HEXISTS", db, key))
	if err != nil {
		log.Warnf("redis: HEXISTS failed (%s)", err)
	}
	return ret, err
}

func (r *RedisStorage) HashLen(db string) (int, error) {
	ret, err := redis.Int(r.Do("HLEN", db))
	if err != nil {
		log.Warnf("redis: HLEN failed (%s)", err)
	}
	return ret, err
}

func (r *RedisStorage) HashSetNotExist(db string, key string, val []byte) (int, error) {
	ret, err := redis.Int(r.Do("HSETNX", db, key, val))
	if err != nil {
		log.Warnf("redis: HSETNX failed (%s)", err)
	}
	return ret, err
}

func (r *RedisStorage) HashDel(db string, key string) (int, error) {
	ret, err := redis.Int(r.Do("HDEL", db, key))
	if err != nil {
		log.Warnf("redis: HDEL failed (%s)", err)
	}
	return ret, err
}

func (r *RedisStorage) HashIncrBy(db string, key string, val int64) (int64, error) {
	ret, err := redis.Int64(r.Do("HINCRBY", db, key, val))
	if err != nil {
		log.Warnf("redis: HINCRBY failed, (%s)", err)
	}
	return ret, err
}

func (r *RedisStorage) SetNotExist(key string, val []byte) (int, error) {
	ret, err := redis.Int(r.Do("SETNX", key, val))
	if err != nil {
		log.Warnf("redis: SETNX failed (%s)", err)
	}
	return ret, err
}

func (r *RedisStorage) IncrBy(key string, val int64) (int64, error) {
	ret, err := redis.Int64(r.Do("INCRBY", key, val))
	if err != nil {
		log.Warnf("redis: INCRBY failed, (%s)", err)
	}
	return ret, err
}

func (r *RedisStorage) SetAdd(key string, val string) (int, error) {
	ret, err := redis.Int(r.Do("SADD", key, val))
	if err != nil {
		log.Warnf("redis: SADD failed, (%s)", err)
	}
	return ret, err
}

func (r *RedisStorage) SetDel(key string, val string) (int, error) {
	ret, err := redis.Int(r.Do("SREM", key, val))
	if err != nil {
		log.Warnf("redis: SREM failed, (%s)", err)
	}
	return ret, err
}

func (r *RedisStorage) SetIsMember(key string, val string) (int, error) {
	ret, err := redis.Int(r.Do("SISMEMBER", key, val))
	if err != nil {
		log.Warnf("redis: SISMEMBER failed, (%s)", err)
	}
	return ret, err

}

func (r *RedisStorage) SetMembers(key string) ([]string, error) {
	ret, err := redis.Strings(r.Do("SMEMBERS", key))
	if err != nil {
		log.Warnf("redis: SMEMBERS failed, (%s)", err)
	}
	return ret, err
}

func (r *RedisStorage) KeyExpire(key string, ttl int32) (int, error) {
	ret, err := redis.Int(r.Do("EXPIRE", key, ttl))
	if err != nil {
		log.Warnf("redis: EXPIRE failed, (%s)", err)
	}
	return ret, err
}
