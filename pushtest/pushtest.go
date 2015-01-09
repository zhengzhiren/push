package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	//"strings"
	"github.com/chenyf/push/comet"
	log "github.com/cihub/seelog"
	"strconv"
)

type Config struct {
	Server    string        `json:"server"`
	Name      string        `json:"name"`
	HeartBeat time.Duration `json:"heartbeat"`
	AppIds    []string      `json:"appids"`
	Interval  time.Duration `json:interval"`
}

type RegApp struct {
	RegId string `json:"regid"`
	Pkg   string `json:"pkg"`
}

type Client struct {
	index   int
	broken  bool
	Init    bool
	DevId   string
	conn    *net.TCPConn
	outMsgs chan *comet.Message
	nextSeq uint32
	ctrl    chan bool // notify sendout routing to quit when close connection
	RegApps map[string]*RegApp
}

func NewClient(index int, devId string, conn *net.TCPConn, outMsgs chan *comet.Message) *Client {
	return &Client{
		index:   index,
		broken:  false,
		Init:    false,
		DevId:   devId,
		conn:    conn,
		outMsgs: outMsgs,
		RegApps: make(map[string]*RegApp),
	}
}

func (client *Client) HandleMessage() bool {
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
		log.Warnf("deserialize header failed, %s", err)
		return false
	}
	if header.Len <= 0 {
		return true
	}

	data := make([]byte, header.Len)
	if _, err := io.ReadFull(client.conn, data); err != nil {
		log.Infof("read from server failed: (%v)", err)
		return false
	}

	log.Infof("client %d: recv: (%d) (%s)", client.index, header.Type, string(data))
	if header.Type == comet.MSG_REGISTER_REPLY {
		var reply comet.RegisterReplyMessage
		if err := json.Unmarshal(data, &reply); err != nil {
			log.Infof("invalid reply, not JSON\n")
			return true
		}
		_, ok := client.RegApps[reply.AppId]
		if !ok {
			client.RegApps[reply.AppId] = &RegApp{
				RegId: reply.RegId,
				Pkg:   reply.Pkg,
			}
		}

	} else if header.Type == comet.MSG_PUSH {
		var request comet.PushMessage
		if err := json.Unmarshal(data, &request); err != nil {
			log.Infof("invalid reply, not JSON")
			return true
		}
		regapp, ok := client.RegApps[request.AppId]
		if !ok {
			//log.Infof("Unknown appid (%s)", request.AppId)
			return true
		}
		response := comet.PushReplyMessage{
			MsgId: request.MsgId,
			AppId: request.AppId,
			RegId: regapp.RegId,
		}
		b, _ := json.Marshal(response)
		client.SendMessage(comet.MSG_PUSH_REPLY, header.Seq, b)
	} else if header.Type == comet.MSG_INIT_REPLY {
		var reply comet.InitReplyMessage
		json.Unmarshal(data, &reply)
		for _, app := range reply.Apps {
			/*
				if _, ok := client.RegApps[app.AppId]; !ok {
					client.RegApps[app.AppId] = &RegApp{
						RegId : app.RegId,
						Pkg : app.Pkg,
					}
				}
			*/
			sendUnregister(client, app.AppId, app.RegId, "")
		}
		client.Init = true
	}
	return true
}

func (client *Client) SendMessage(msgType uint8, seq uint32, body []byte) (uint32, bool) {
	bodylen := 0
	if body != nil {
		bodylen = len(body)
	}
	header := &comet.Header{
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

func sendInit(client *Client, config *Config) {
	init_msg := comet.InitMessage{
		DevId: client.DevId,
		Sync:  1,
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

func sendUnregister(client *Client, appid string, regid string, token string) {
	reg_msg := comet.UnregisterMessage{
		AppId:  appid,
		AppKey: "",
		RegId:  regid,
	}
	b2, _ := json.Marshal(reg_msg)
	client.SendMessage(comet.MSG_UNREGISTER, 0, b2)
}

func runClient(index int, config *Config, wg *sync.WaitGroup, ctrl chan bool, local, server *net.TCPAddr) {
	log.Infof("client %d: enter routine", index)
	conn, err := net.DialTCP("tcp", nil, server)
	if err != nil {
		log.Warnf("client %d: connect to server failed: %v", index, err)
		return
	}
	log.Infof("client %d: connected to server (%s->%s)", index, conn.LocalAddr(), conn.RemoteAddr())
	conn.SetNoDelay(true)
	defer conn.Close()

	outMsg := make(chan *comet.Message, 10)
	ctrl2 := make(chan bool, 1)
	client := NewClient(index, fmt.Sprintf("perftest-%s-%d", config.Name, index), conn, outMsg)

	sendInit(client, config)

	wg.Add(1)
	go func() {
		// output routine
		heartbeat := make([]byte, 1)
		heartbeat[0] = 0
		for {
			select {
			case <-ctrl2:
				log.Infof("client %d: leave output routine", index)
				return
			case msg := <-outMsg:
				b, _ := msg.Header.Serialize()
				if _, err := conn.Write(b); err != nil {
					log.Warnf("client %d: sendout header failed, %s", index, err)
					client.broken = true
					return
				}
				if msg.Data != nil {
					if _, err := conn.Write(msg.Data); err != nil {
						log.Warnf("client %d: sendout body failed, %s", index, err)
						client.broken = true
						return
					}
				}
				log.Infof("client %d: send: (%d) (%s)", index, msg.Header.Type, msg.Data)
			case <-time.After(config.HeartBeat * time.Second):
				conn.Write(heartbeat)
			}
		}
	}()

	time.Sleep(3 * time.Second)
	registered := false
	for {
		// main loop
		select {
		case <-ctrl:
			close(ctrl2)
			wg.Done()
			log.Infof("client %d: leave routine", index)
			return
		default:
		}
		if client.broken {
			close(ctrl2)
			wg.Done()
			log.Infof("client %d: leave routine", index)
			return
		}
		if !registered && client.Init {
			for _, appid := range config.AppIds {
				sendRegister(client, appid, "", "")
				time.Sleep(1 * time.Second)
			}
			registered = true
		}

		if ok := client.HandleMessage(); !ok {
			close(ctrl2)
			wg.Done()
			log.Infof("client %d: leave routine", index)
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
}

func main() {
	cfgfile := "conf.json"
	begin := 0
	count := 1
	if len(os.Args) >= 3 {
		begin, _ = strconv.Atoi(os.Args[1])
		count, _ = strconv.Atoi(os.Args[2])
	}
	r, err := os.Open(cfgfile)
	if err != nil {
		fmt.Printf("failed to open config file %s", cfgfile)
		os.Exit(1)
	}

	var config Config
	decoder := json.NewDecoder(r)
	err = decoder.Decode(&config)
	if err != nil {
		fmt.Printf("invalid config file")
		os.Exit(1)
	}

	logger, err := log.LoggerFromConfigAsFile("log.xml")
	if err != nil {
		fmt.Printf("Load log config failed: (%s)\n", err)
		os.Exit(1)
	}
	log.ReplaceLogger(logger)

	server, _ := net.ResolveTCPAddr("tcp4", config.Server)
	wg := &sync.WaitGroup{}
	c := make(chan os.Signal, 1)
	ctrl := make(chan bool, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

	running := true
	wg.Add(1)
	go func() {
		sig := <-c
		log.Infof("Received signal '%v', exiting\n", sig)
		running = false
		close(ctrl)
		wg.Done()
	}()

	for n := begin; n < begin+count; n++ {
		if !running {
			break
		}
		go runClient(n, &config, wg, ctrl, nil, server)
		time.Sleep(config.Interval * time.Millisecond)
	}
	wg.Wait()
}
