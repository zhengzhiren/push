package main

import (
	"net"
	"log"
	"io"
	"os"
	"fmt"
	"time"
	"encoding/json"
	//"strings"
	"github.com/chenyf/push/comet"
)

type CommandRequest struct {
	Uid		string	`json:"uid"`
	Cmd		string	`json:"cmd"`
}

type CommandResponse struct {
	Status		int		`json:"status"`
	Error		string	`json:"error"`
	Response	string	`json:"response"`
}

func sendInit(conn *net.TCPConn, devid string) {
	init_msg := comet.InitMessage{
		DeviceId : devid,
	}

	b2, _ := json.Marshal(init_msg)
	header := comet.Header{
		Type: comet.MSG_INIT,
		Ver: 0,
		Seq: 0,
		Len: uint32(len(b2)),
	}
	b1, _ := header.Serialize()
	n1, _ := conn.Write(b1)
	n2, _ := conn.Write(b2)
	log.Printf("write out %d, %d", n1, n2)
}

func sendRegister(conn *net.TCPConn, appid string, appkey string, regid string) {
	reg_msg := comet.RegisterMessage{
		AppId : appid,
		AppKey : appkey,
		RegId : regid,
	}

	b2, _ := json.Marshal(reg_msg)
	header := comet.Header{
		Type: comet.MSG_REGISTER,
		Ver: 0,
		Seq: 0,
		Len: uint32(len(b2)),
	}
	b1, _ := header.Serialize()
	n1, _ := conn.Write(b1)
	n2, _ := conn.Write(b2)
	log.Printf("write out %d, %d", n1, n2)
}

func main() {
	if len(os.Args) <= 4 {
		log.Printf("Usage: server_addr devid appid regid")
		return
	}
	svraddr := os.Args[1]
	devid := os.Args[2]
	appid := os.Args[3]
	regid := os.Args[4]

	addr, _ := net.ResolveTCPAddr("tcp4", svraddr)
	conn, err := net.DialTCP("tcp", nil, addr)
	if err != nil {
		log.Printf("connect to server failed: %v", err)
		return
	}
	conn.SetNoDelay(true)
	defer conn.Close()

	sendInit(conn, devid)
	time.Sleep(10)
	sendRegister(conn, appid, "mykey1", regid)

	outMsg := make(chan *comet.Message, 10)
	go func(out chan *comet.Message) {
		timer := time.NewTicker(60*time.Second)
		hb := comet.Header{
			Type: comet.MSG_HEARTBEAT,
			Ver: 0,
			Seq: 0,
			Len: 0,
		}
		heartbeat, _ := hb.Serialize()
		for {
			select {
			//case <- done:
			//	break
			case msg := <-out:
				b, _ := msg.Header.Serialize()
				conn.Write(b)
				conn.Write(msg.Data)
				log.Printf("send: (%d) (%d) (%s)", msg.Header.Type, msg.Header.Len, msg.Data)
			case <- timer.C:
				conn.Write(heartbeat)
			}
		}
	}(outMsg)

	for {
		headSize := 10
		buf := make([]byte, headSize)
		if _, err := io.ReadFull(conn, buf); err != nil {
			log.Printf("read message len failed, (%v)\n", err)
			return
		}

		var header comet.Header
		if err := header.Deserialize(buf); err != nil {
			return
		}

		data := make([]byte, header.Len)
		if _, err := io.ReadFull(conn, data); err != nil {
			log.Printf("read from client failed: (%v)", err)
			return
		}

		log.Printf("recv: (%d) (%d) (%s)", header.Type, header.Len, string(data))
		if header.Type == comet.MSG_PUSH {
			var request comet.PushMessage
			if err := json.Unmarshal(data, &request); err != nil {
				log.Printf("invalid request, not JSON\n")
				return
			}
			response := comet.PushReplyMessage{
				MsgId : request.MsgId,
				AppId : appid,
				RegId : fmt.Sprintf("%s_%s", devid, appid),
			}

			b, _ := json.Marshal(response)
			reply_header := comet.Header{
				Type: comet.MSG_PUSH_REPLY,
				Ver: 0,
				Seq: header.Seq,
				Len: uint32(len(b)),
			}
			reply_msg := &comet.Message{
				Header: reply_header,
				Data: b,
			}
			outMsg <- reply_msg
		}
	}
}

