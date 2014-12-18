package storage

import (
	"github.com/chenyf/push/conf"
)

type RawMessage struct {
	AppSec     string `json:"appsec,omitempty"`
	Token      string `json:"token,omitempty"`
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
		Topic  string   `json:"topic,omitempty"`
	} `json:"push_params"`
	Content      string `json:"content,omitempty"`
	Notification struct {
		Title     string `json:"title"`
		Desc      string `json:"desc,omitempty"`
		Type      int    `json:"type,omitempty"`
		SoundUri  string `json:"sound_uri,omitempty"`
		Action    int    `json:"action,omitempty"`
		IntentUri string `json:"intent_uri,omitempty"`
		WebUri    string `json:"web_uri,omitempty"`
	} `json:"notification,omitempty"`
	Options struct {
		TTL int64 `json:"ttl,omitempty"`
		TTS int64 `json:"tts,omitempty"`
	} `json:"options"`
	SendId string `json:"sendid,omitempty"`
}

type RawApp struct {
	Pkg    string `json:"pkg"`
	UserId string `json:"userid"`
	Name   string `json:"name"`
	Mobile string `json:"mobile"`
	Email  string `json:"email"`
	Desc   string `json:"desc"`
	AppKey string `json:"appkey,omitempty"`
	AppSec string `json:"appsec,omitempty"`
}

type Storage interface {
	GetOfflineMsgs(appId string, regId string, ctime int64) []*RawMessage
	GetRawMsg(appId string, msgId int64) *RawMessage

	AddDevice(serverName, devId string) error
	RemoveDevice(serverName, devId string) error
	// Get all comet server names
	GetServerNames() ([]string, error)
	// Get all device Ids on a server
	GetDeviceIds(serverName string) ([]string, error)

	//
	// check if the device Id exists, return the server name
	//
	CheckDevice(devId string) (string, error)
	RefreshDevices(serverName string, timeout int) error
	InitDevices(serverName string) error

	//
	// statistics
	//
	GetStatsServices() ([]string, error)

	IncCmd(service string) error
	GetStatsCmd(service string) (int, error)

	IncCmdSuccess(service string) error
	GetStatsCmdSuccess(service string) (int, error)

	IncCmdTimeout(service string) error
	GetStatsCmdTimeout(service string) (int, error)

	IncCmdOffline(service string) error
	GetStatsCmdOffline(service string) (int, error)

	IncCmdInvalidService(service string) error
	GetStatsCmdInvalidService(service string) (int, error)

	IncCmdOtherError(service string) error
	GetStatsCmdOtherError(service string) (int, error)

	IncQueryOnlineDevices() error
	GetStatsQueryOnlineDevices() (int, error)

	IncQueryDeviceInfo() error
	GetStatsQueryDeviceInfo() (int, error)

	IncReplyTooLate() error
	GetStatsReplyTooLate() (int, error)

	ClearStats() error

	HashGetAll(db string) ([]string, error)
	HashGet(db string, key string) ([]byte, error)
	HashSet(db string, key string, val []byte) (int, error)
	HashExists(db string, key string) (int, error)
	HashSetNotExist(db string, key string, val []byte) (int, error)
	HashDel(db string, key string) (int, error)
	HashIncrBy(db string, key string, val int64) (int64, error)
	HashLen(db string) (int, error)

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
