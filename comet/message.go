package comet

import (
	"bytes"
	"encoding/binary"
)

type RawMessage struct {
	UserName string `json:"username"`
	AppKey string `json:"appkey"`
	//sendType 1广播 2单播 3组播(tag alias)
	//sendTypeParams 1空(dev_list) 2devid 3tag_list alias_list
	SendType int `json:"send_type"`
	SendParams interface{} `json:"send_params"`
	//1通知 2消息 
	MsgType int `json:"msg_type"`
	MsgTitle string `json:"msg_title"`
	MsgContent string `json:"msg_content"`
	ClientPlatform int `json:"client_platform"`
        //sendMtdType 1实时 2定时 3离线
	//sendMtdParams 1空 2定时时间 3离线时间
	SendMthType int `json:"smth_type"`
	SendMthParams interface{} `json:"smth_params"`
	CreateTime int64 `json:"ctime"`
	CustomContent string `json:"custom_content"`
}

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

