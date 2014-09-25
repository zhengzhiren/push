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
	MSG_REQUEST				= uint8(10)
	MSG_REQUEST_REPLY		= uint8(11)
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

