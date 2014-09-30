package storage

import (
)

type AppInfo struct{
	LastMsgId	int64	`json:"last_msgid"`
}

type RawMessage struct {
	MsgId		int64		`json:"msgid"`
	AppId		string		`json:"appid"`
	CTime		int64		`json:"ctime"`
	Platform	string		`json:"platform"`
	MsgType		int			`json:"msg_type"` //消息类型: 1通知 2消息
	PushType	int			`json:"push_type"`//发送类型: 1广播 2单播 3组播
	Content		string		`json:"content"`
	Options		interface{}	`json:"options"` //可选 title, ttl(消息生命周期), push_params(单播devid 组播tag list,alias list), timing(定时发送时间), custom_content等等
}

type Storage interface {
	GetOfflineMsgs(appId string, ctime int64) []*RawMessage
	GetRawMsg(appId string, msgId int64) *RawMessage
	GetApp(appId string, regId string) (*AppInfo)
	UpdateApp(appId string, regId string, msgId int64) error
	AddDevice(devId string) bool
}

var (
	StorageInstance Storage = newRedisStorage()
)

