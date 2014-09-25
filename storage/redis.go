package storage

import (
	//"time"
	"github.com/chenyf/push/error"
)

type RedisStorage struct {

}

// 从存储后端获取 > 指定时间的所有消息
func (storage RedisStorage)GetOfflineMsgs(appId string, ctime int64) []*PushMessage {
	msg_list := []*PushMessage{}
	return msg_list
}

// 从存储后端获取指定消息
func (storage RedisStorage)GetMsg(appId string, msgId int64) *PushMessage {
	return nil
}

func (storage RedisStorage)GetApp(regId string) (*AppInfo) {
	return nil
}

func (storage RedisStorage)AddApp(regId string, appId string, appKey string, devId string) error {
	return &pusherror.PushError{"add failed"}
}

