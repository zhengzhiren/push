package storage

import (
)

type PushMessage struct {
	CreateTime	int64
	Body []byte
}

type AppInfo struct{
	LastMsgId	int64	`json:"last_msgid"`
}
type Storage interface {
	GetOfflineMsgs(appId string, ctime int64) []string
	GetMsg(appId string, msgId int64) string
	GetApp(appId string, regId string) (*AppInfo, error)
	UpdateApp(appId string, regId string, msgId int64) error
}

var (
	StorageInstance Storage = newRedisStorage()
)

