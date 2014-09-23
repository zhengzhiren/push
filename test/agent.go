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

type InitMessage struct {
	Id	string	`json:"id"`
}

func main() {
	if len(os.Args) <= 2 {
		log.Printf("Usage: server_addr devid")
		return
	}
	svraddr := os.Args[1]
	devid := os.Args[2]

	addr, _ := net.ResolveTCPAddr("tcp4", svraddr)
	conn, err := net.DialTCP("tcp", nil, addr)
	if err != nil {
		log.Printf("connect to server failed: %v", err)
		return
	}
	conn.SetNoDelay(true)
	defer conn.Close()

	init_msg := InitMessage{
		Id : devid,
	}
	b2, _ := json.Marshal(init_msg)

	header := comet.Header{
		Type: comet.MSG_INIT,
		Ver: 0,
		Seq: 0,
		Len: uint32(len(b2)),
	}

	b, _ := header.Serialize()
	n1, _ := conn.Write(b)
	n2, _ := conn.Write(b2)
	log.Printf("write out %d, %d", n1, n2)

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

		log.Printf("recv from server (%s)", string(data))
		if header.Type == comet.MSG_REQUEST {

			var request CommandRequest
			if err := json.Unmarshal(data, &request); err != nil {
				log.Printf("invalid request, not JSON\n")
				return
			}

			fmt.Printf("UID: (%s)\n", request.Uid)

			response := CommandResponse{
				Status: 0,
				Error : "OK",
				Response : fmt.Sprintf("Sir, %s got it!", devid),
			}

			b, _ := json.Marshal(response)
			reply_header := comet.Header{
				Type: comet.MSG_REQUEST_REPLY,
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

