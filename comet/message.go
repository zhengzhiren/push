package comet

import (
	"bytes"
	"encoding/binary"
)

type Header struct {
	Type	uint8
	Ver		uint8
	Seq		uint32
	Len		uint32
}

type Message struct {
	Header	Header
	Data	[]byte
}

const (
	HEADER_SIZE				= uint32(10)
	MSG_HEARTBEAT			= uint8(0)
	MSG_INIT				= uint8(1)
	MSG_INIT_REPLY			= uint8(2)
	MSG_REGISTER			= uint8(3)
	MSG_REGISTER_REPLY		= uint8(4)
	MSG_UNREGISTER			= uint8(5)
	MSG_UNREGISTER_REPLY	= uint8(6)
	MSG_PUSH				= uint8(10)
	MSG_PUSH_REPLY			= uint8(11)
)

// msg to byte
func (header *Header)Serialize() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.BigEndian, *header); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// byte to msg
func (header *Header)Deserialize(b []byte) (error) {
	buf := bytes.NewReader(b)
	if err := binary.Read(buf, binary.BigEndian, header); err != nil {
		return err
	}
	return nil
}

type InitMessage struct {
	DeviceId	string				`json:"devid"`
	Apps		[]RegisterMessage	`json:"apps"`
}
type InitReplyMessage struct {
	Result	string `json:"result"`
}
type RegisterMessage struct{
	AppId	string	`json:"appid"`
	AppKey	string	`json:"appkey"`
	RegId	string	`json:"regid"`
	Token	string	`json:"token"`
}
type RegisterReplyMessage struct{
	AppId	string	`json:"appid"`
	RegId	string	`json:"regid"`
	Result	int		`json:"result"`
}
type UnregisterMessage struct{
	AppId	string	`json:"appid"`
	AppKey	string	`json:"appkey"`
	RegId	string	`json:"regid"`
	Token	string	`json:"token"`
}
type UnregisterReplyMessage struct{
	AppId	string	`json:"appid"`
	RegId	string	`json:"regid"`
	Result	int		`json:"result"`
}
type PushMessage struct {
	MsgId		int64	`json:"msgid"`
	AppId		string	`json:"appid"`
	Type		int		`json:"type"`  //1: notification  2:app message
	Content		string	`json:"content"`
}
type PushReplyMessage struct {
	MsgId	int64	`json:"msgid"`
	AppId	string	`json:"appid"`
	RegId	string	`json:"regid"`
}

