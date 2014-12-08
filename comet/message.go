package comet

import (
	"bytes"
	"encoding/binary"
)

type Header struct {
	Type uint8
	Ver  uint8
	Seq  uint32
	Len  uint32
}

type Message struct {
	Header Header
	Data   []byte
}

const (
	HEADER_SIZE           = uint32(10)
	MSG_HEARTBEAT         = uint8(0)
	MSG_INIT              = uint8(1)
	MSG_INIT_REPLY        = uint8(2)
	MSG_REGISTER          = uint8(3)
	MSG_REGISTER_REPLY    = uint8(4)
	MSG_UNREGISTER        = uint8(5)
	MSG_UNREGISTER_REPLY  = uint8(6)
	MSG_PUSH              = uint8(10)
	MSG_PUSH_REPLY        = uint8(11)
	MSG_SUBSCRIBE         = uint8(12)
	MSG_SUBSCRIBE_REPLY   = uint8(13)
	MSG_UNSUBSCRIBE       = uint8(14)
	MSG_UNSUBSCRIBE_REPLY = uint8(15)
	MSG_GET_TOPICS        = uint8(16)
	MSG_GET_TOPICS_REPLY  = uint8(17)
	MSG_CMD               = uint8(20)
	MSG_CMD_REPLY         = uint8(21)
	MSG_REGISTER2         = uint8(23)
	MSG_REGISTER2_REPLY   = uint8(24)
	MSG_UNREGISTER2       = uint8(25)
	MSG_UNREGISTER2_REPLY = uint8(26)
)

// msg to byte
func (header *Header) Serialize() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.BigEndian, *header); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// byte to msg
func (header *Header) Deserialize(b []byte) error {
	buf := bytes.NewReader(b)
	if err := binary.Read(buf, binary.BigEndian, header); err != nil {
		return err
	}
	return nil
}

type Base1 struct {
	AppId string `json:"appid"`
	RegId string `json:"regid"`
}
type InitMessage struct {
	DevId string  `json:"devid"`
	Sync  int     `json:"sync,omitempty"`
	Apps  []Base1 `json:"apps,omitempty"`
}

type Base2 struct {
	AppId string `json:"appid"`
	RegId string `json:"regid"`
	Pkg   string `json:"pkg"`
}
type InitReplyMessage struct {
	Result int     `json:"result"`
	Apps   []Base2 `json:"apps,omitempty"`
}
type RegisterMessage struct {
	AppId  string `json:"appid"`
	AppKey string `json:"appkey"`
	RegId  string `json:"regid"`
	Uid    string `json:"uid,omitempty"`
	Token  string `json:"token,omitempty"`
}
type RegisterReplyMessage struct {
	Result int    `json:"result"`
	AppId  string `json:"appid"`
	Pkg    string `json:"pkg"`
	RegId  string `json:"regid"`
}
type UnregisterMessage struct {
	AppId  string `json:"appid"`
	AppKey string `json:"appkey"`
	RegId  string `json:"regid"`
}
type UnregisterReplyMessage struct {
	Result int    `json:"result"`
	AppId  string `json:"appid"`
}
type PushMessage struct {
	MsgId   int64  `json:"msgid"`
	AppId   string `json:"appid"`
	Type    int    `json:"type"` //1: notification  2:app message
	Content string `json:"content"`
}
type PushReplyMessage struct {
	MsgId int64  `json:"msgid"`
	AppId string `json:"appid"`
	RegId string `json:"regid"`
}
type SubscribeMessage struct {
	AppId string `json:"appid"`
	RegId string `json:"regid"`
	Topic string `json:"topic"`
}
type SubscribeReplyMessage struct {
	Result int    `json:"result"`
	AppId  string `json:"appid"`
	RegId  string `json:"regid"`
}
type UnsubscribeMessage struct {
	AppId string `json:"appid"`
	RegId string `json:"regid"`
	Topic string `json:"topic"`
}
type UnsubscribeReplyMessage struct {
	Result int    `json:"result"`
	AppId  string `json:"appid"`
	RegId  string `json:"regid"`
}
type GetTopicsMessage struct {
	AppId string `json:"appid"`
	RegId string `json:"regid"`
}
type GetTopicsReplyMessage struct {
	Result int    `json:"result"`
	AppId string `json:"appid"`
	RegId string `json:"regid"`
	Topics []string `json:"topics"`
}
type CommandMessage struct {
	Service string `json:"service"`
	Cmd     string `json:"cmd"`
}
type CommandReplyMessage struct {
	Status int    `json:"status"`
	Result string `json:"result"`
}

type Register2Message struct {
	Pkg     string   `json:"pkg"`
	Uid     string   `json:"uid,omitempty"`
	SendIds []string `json:"sendids"`
}
type Register2ReplyMessage struct {
	Result int    `json:"result"`
	AppId  string `json:"appid"`
	Pkg    string `json:"pkg"`
	RegId  string `json:"regid"`
}
type Unregister2Message struct {
	Pkg     string `json:"pkg"`
	Uid     string `json:"uid,omitempty"`
	SendId  string `json:"sendid"`  // [all]
}
type Unregister2ReplyMessage struct {
	Result int    `json:"result"`
	AppId  string `json:"appid"`
	Pkg    string `json:"pkg"`
	SenderCnt int `json:"sendercnt"`
}

