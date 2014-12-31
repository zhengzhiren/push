package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"encoding/json"
	"time"
	//"strings"
	"github.com/chenyf/push/comet"
	log "github.com/cihub/seelog"
)

type Config struct {
	Server string `json:"server"`
	AppIds []string `json:"appids"`
	Count int `json:"count"`
}

type Client struct {
	DevId   string
	conn    *net.TCPConn 
	outMsgs chan *comet.Message
	nextSeq uint32
	ctrl    chan bool // notify sendout routing to quit when close connection
	Broken  bool
}

func NewClient(devId string, conn *net.TCPConn, outMsgs chan *comet.Message) *Client {
	return &Client{
		DevId : devId,
		conn : conn,
		outMsgs : outMsgs,
	}
}

func (client *Client)HandleMessage() bool {
	client.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	headSize := 10
	buf := make([]byte, headSize)
	if _, err := io.ReadFull(client.conn, buf); err != nil {
		if e, ok := err.(*net.OpError); ok && e.Timeout() {
			return true
		}
		log.Infof("read failed, (%v)\n", err)
		return false
	}

	var header comet.Header
	if err := header.Deserialize(buf); err != nil {
		return false
	}
	if header.Len <= 0 {
		return true
	}

	data := make([]byte, header.Len)
	if _, err := io.ReadFull(client.conn, data); err != nil {
		log.Infof("read from client failed: (%v)", err)
		return false
	}

	log.Infof("recv: (%d) (%d) (%s)", header.Type, header.Len, string(data))
	if header.Type == comet.MSG_REGISTER_REPLY {
		var reply comet.RegisterReplyMessage
		if err := json.Unmarshal(data, &reply); err != nil {
			log.Infof("invalid request, not JSON\n")
			return true
		}
		regid := reply.RegId
		log.Infof("got regid (%s)", regid)
	} else if header.Type == comet.MSG_PUSH {
		var request comet.PushMessage
		if err := json.Unmarshal(data, &request); err != nil {
			log.Infof("invalid request, not JSON\n")
			return true
		}
		response := comet.PushReplyMessage{
			MsgId: request.MsgId,
			AppId: request.AppId,
			RegId: "",
		}
		b, _ := json.Marshal(response)
		client.SendMessage(comet.MSG_PUSH_REPLY, header.Seq, b)
	}
	return true
}

func (client *Client)SendMessage(msgType uint8, seq uint32, body []byte) (uint32, bool) {
	bodylen := 0
	if body != nil {
		bodylen = len(body)
	}
	header := comet.Header{
		Type: msgType,
		Ver:  0,
		Seq:  seq,
		Len:  uint32(bodylen),
	}
	msg := &comet.Message{
		Header: header,
		Data:   body,
	}
	client.outMsgs <- msg
	return seq, true
}

func sendInit(client *Client) {
	init_msg := comet.InitMessage{
		DevId: client.DevId,
		Sync: 1,
	}
	b2, _ := json.Marshal(init_msg)
	client.SendMessage(comet.MSG_INIT, 0, b2)
}

func sendRegister(client *Client, appid string, regid string, token string) {
	reg_msg := comet.RegisterMessage{
		AppId:  appid,
		AppKey: "",
		RegId:  regid,
		Token:  token,
	}

	b2, _ := json.Marshal(reg_msg)
	client.SendMessage(comet.MSG_REGISTER, 0, b2)
}

func runClient(config *Config, index int, wg *sync.WaitGroup, ctrl chan bool, addr *net.TCPAddr) {
	log.Infof("enter client routine")
	conn, err := net.DialTCP("tcp", nil, addr)
	if err != nil {
		log.Infof("connect to server failed: %v", err)
		return
	}
	log.Infof("connected to server")
	conn.SetNoDelay(true)
	defer conn.Close()

	outMsg := make(chan *comet.Message, 10)
	ctrl2 := make(chan bool, 1)
	client := NewClient(fmt.Sprintf("testdevid_%d", index), conn, outMsg)

	sendInit(client)

	wg.Add(1)
	go func() {
	// output routine
		heartbeat := make([]byte, 1)
		heartbeat[0] = 0
		for {
			select {
			case <-ctrl2:
				log.Infof("leave output routine")
				return
			case msg := <-outMsg:
				b, _ := msg.Header.Serialize()
				conn.Write(b)
				conn.Write(msg.Data)
				log.Infof("send: (%d) (%d) (%s)", msg.Header.Type, msg.Header.Len, msg.Data)
			case <-time.After(50*time.Second):
				conn.Write(heartbeat)
			}
		}
	}()

	time.Sleep(3)
	for _, appid := range(config.AppIds) {
		sendRegister(client, appid, "", "")
		time.Sleep(1)
	}

	for {
	// main loop
		select {
		case <-ctrl:
			close(ctrl2)
			wg.Done()
			log.Infof("leave client routine")
			return
		default:
		}
		if ok := client.HandleMessage(); !ok {
			close(ctrl2)
			wg.Done()
			log.Infof("leave client routine")
			return
		}
		time.Sleep(1)
	}
}


func main() {
	if len(os.Args) <= 1 {
		log.Infof("Usage: cfgfile")
		return
	}
	cfgfile := os.Args[1]
	r, err := os.Open(cfgfile)
	if err != nil {
		return
	}
	decoder := json.NewDecoder(r)

	var config Config
	err = decoder.Decode(&config)
	if err != nil {
		return
	}

	addr, _ := net.ResolveTCPAddr("tcp4", config.Server)

	wg := &sync.WaitGroup{}
	c := make(chan os.Signal, 1)
	ctrl := make(chan bool, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

	wg.Add(1)
	go func() {
		sig := <-c
		log.Infof("Received signal '%v', exiting\n", sig)
		close(ctrl)
		wg.Done()
	}()

	for n := 0; n < config.Count; n++ {
		go runClient(&config, n, wg, ctrl, addr)
		time.Sleep(2)
	}
	wg.Wait()

}



