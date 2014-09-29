package storage

import (
	"github.com/chenyf/push/message"
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
	GetRawMsg(appId string, msgId int64) *message.RawMessage
	GetApp(appId string, regId string) (*AppInfo)
	UpdateApp(appId string, regId string, msgId int64) error
}

var (
	StorageInstance Storage = newRedisStorage()
)

