package storage

import (
)

type PushMessage struct {
	CreateTime	int64
	Body []byte
}

type AppInfo struct{
	AppKey		string
	DevId		string
	LastMsgId	int64
}
type Storage interface {
	// 获取离线消息
	GetOfflineMsgs(appId string, lastMsgId int64) []*PushMessage
	// 获取指定消息
	GetMsg(appId string, msgId int64) *PushMessage

	// 获取注册app的持久化信息
	GetApp(appId string, regId string) (*AppInfo)

	// 增加一个注册app
	AddApp(appId string, regId string, appKey string, devId string) error
}

var (
	StorageInstance Storage = RedisStorage{}
)

