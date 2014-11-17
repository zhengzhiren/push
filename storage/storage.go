package storage

import (
	"github.com/chenyf/push/conf"
)

type RawMessage struct {
	Token      string `json:"token"`
	UserId     string `json:"userid"`
	AppSec     string `json:"appsec,omitempty"`
	MsgId      int64  `json:"msgid"`
	AppId      string `json:"appid"`
	Pkg        string `json:"pkg"`
	CTime      int64  `json:"ctime"`
	Platform   string `json:"platform,omitempty"`
	MsgType    int    `json:"msg_type"`
	PushType   int    `json:"push_type"`
	PushParams struct {
		RegId  []string `json:"regid,omitempty"`
		UserId []string `json:"userid,omitempty"`
		DevId  []string `json:"devid,omitempty"`
	} `json:"push_params"`
	Content string `json:"content"`
	Options struct {
		TTL int64 `json:"ttl,omitempty"`
	} `json:"options"`
}

type RawApp struct {
	Pkg    string `json:"pkg"`
	UserId string `json:"userid"`
	AppKey string `json:"appkey,omitempty"`
	AppSec string `json:"appsec,omitempty"`
}

type Storage interface {
	GetOfflineMsgs(appId string, regId string, ctime int64) []*RawMessage
	GetRawMsg(appId string, msgId int64) *RawMessage

	AddDevice(serverName, devId string) error
	RemoveDevice(serverName, devId string) error
	IsDeviceExist(devId string) (bool, error)
	RefreshDevices(serverName string, timeout int) error

	HashGetAll(db string) ([]string, error)
	HashGet(db string, key string) ([]byte, error)
	HashSet(db string, key string, val []byte) (int, error)
	HashExists(db string, key string) (int, error)
	HashSetNotExist(db string, key string, val []byte) (int, error)
	HashDel(db string, key string) (int, error)
	HashIncrBy(db string, key string, val int64) (int64, error)

	SetNotExist(key string, val []byte) (int, error)
	IncrBy(key string, val int64) (int64, error)

	SetAdd(key string, val string) (int, error)
	SetDel(key string, val string) (int, error)
	SetIsMember(key string, val string) (int, error)
	SetMembers(key string) ([]string, error)
}

var (
	Instance Storage = nil
)

func NewInstance(config *conf.ConfigStruct) bool {
	Instance = NewRedisStorage(config)
	return true
}
