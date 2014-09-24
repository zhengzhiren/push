package storage

import (
	//"time"
)

type Storage struct {
	Backend	string
}

var (
	StorageInstance *Storage = &Storage{
		Backend : "redis",
	}
)

type PushMessage struct {
	CreateTime	int64
	Body []byte
}

func (storage *Storage)GetOfflineMsgs(appId string, ctime int64) []*PushMessage {
	msg_list := []*PushMessage{}
	return msg_list
}

