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
	GetOfflineMsgs(appId string, ctime int64) []string
	GetMsg(appId string, msgId int64) string
	GetApp(regId string) (*AppInfo)
	AddApp(regId string, appId string, appKey string, devId string) error
}

var (
	StorageInstance Storage = newRedisStorage()
)

