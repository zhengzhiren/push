package storage

import (
)

type PushMessage struct {
	CreateTime	int64
	Body []byte
}

type AppInfo struct{
	LastMsgId	int64
}
type Storage interface {
	GetOfflineMsgs(appId string, ctime int64) []*PushMessage
	GetMsg(appId string, msgId int64) *PushMessage
	GetApp(regId string) (*AppInfo)
	AddApp(regId string, appId string, appKey string, devId string) error
}

var (
	StorageInstance Storage = RedisStorage{
	}
)

