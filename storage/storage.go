package storage

import (
)

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

	AddDevice(devId string) bool
	RemoveDevice(devId string)

	HashGetAll(db string) ([]string, error)
	HashGet(db string, key string) ([]byte, error)
	HashSet(db string, key string, val []byte) (int, error)
	HashSetNotExist(db string, key string, val []byte) (int, error)
	HashDel(db string, key string) (int, error)
	HashIncrBy(db string, key string, val int64) (int64, error)

	SetNotExist(key string, val []byte) (int, error)
	IncrBy(key string, val int64) (int64, error)

	SetAdd(key string, val string) (int, error)
	SetMove(key string, val string) (int, error)
	SetMembers(key string) ([]string, error)
}

var (
	Instance Storage = newRedisStorage()
)

