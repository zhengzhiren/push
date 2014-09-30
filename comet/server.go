package comet

import (
	//"log"
	"io"
	"net"
	"sync"
	"time"
	//"fmt"
	//"strings"
	"encoding/json"
	log "github.com/cihub/seelog"
	"github.com/chenyf/push/storage"
	"github.com/chenyf/push/utils/safemap"
	//"github.com/bitly/go-simplejson"
)

type MsgHandler func(*Client, *Header, []byte)(int)

type Pack struct {
	msg		*Message
	client	*Client
	reply	chan *Message
}

type Client struct {
	devId		string
	regApps		map[string]*App
	outMsgs		chan *Pack
	nextSeq		uint32
	lastActive	time.Time
	ctrl		chan bool
}

type Server struct {
	exitCh         chan bool
	waitGroup      *sync.WaitGroup
	funcMap        map[uint8]MsgHandler
	acceptTimeout  time.Duration
	readTimeout    time.Duration
	writeTimeout   time.Duration
	maxMsgLen      uint32
}

func NewServer() *Server {
	return &Server {
		exitCh:        make(chan bool),
		waitGroup:     &sync.WaitGroup{},
		funcMap:       make(map[uint8]MsgHandler),
		acceptTimeout: 60,
		readTimeout:   60,
		writeTimeout:  60,
		maxMsgLen:     2048,
	}
}

func (client *Client)SendMessage(msgType uint8, body []byte, reply chan *Message) {
	header := Header{
		Type:	msgType,
		Ver:	0,
		Seq:	0,
		Len:	uint32(len(body)),
	}
	msg := &Message{
		Header: header,
		Data:	body,
	}

	pack := &Pack{
		msg: msg,
		client: client,
		reply: reply,
	}
	client.outMsgs <- pack
}

var (
	DevicesMap *safemap.SafeMap = safemap.NewSafeMap()
)

func InitClient(conn *net.TCPConn, devid string) (*Client) {
	client := &Client {
		devId: devid,
		regApps: make(map[string]*App),
		nextSeq: 1,
		lastActive: time.Now(),
		outMsgs: make(chan *Pack, 100),
		ctrl: make(chan bool),
	}
	DevicesMap.Set(devid, client)

	go func() {
		log.Infof("enter send routine")
		for {
			select {
			case pack := <-client.outMsgs:
				seqid := pack.client.nextSeq
				pack.msg.Header.Seq = seqid
				b, _ := pack.msg.Header.Serialize()
				conn.Write(b)
				conn.Write(pack.msg.Data)
				log.Infof("send msg: (%d) (%s)", pack.msg.Header.Type, pack.msg.Data)
				pack.client.nextSeq += 1
				time.Sleep(1*time.Second)
			case <-client.ctrl:
				log.Infof("leave send routine")
				return
			}
		}
	}()
	return client
}

func CloseClient(client *Client) {
	client.ctrl <- true
	DevicesMap.Delete(client.devId)
}

func (this *Server) SetReadTimeout(readTimeout time.Duration) {
	this.readTimeout = readTimeout
}

func (this *Server) SetWriteTimeout(writeTimeout time.Duration) {
	this.writeTimeout = writeTimeout
}

func (this *Server) SetAcceptTimeout(acceptTimeout time.Duration) {
	this.acceptTimeout = acceptTimeout
}

func (this *Server) SetMaxPktLen(maxMsgLen uint32) {
	this.maxMsgLen = maxMsgLen
}

func (this *Server) Init(addr string) (*net.TCPListener, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp4", addr)
	l, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		log.Infof("failed to listen, (%v)", err)
		return nil, err
	}
	this.funcMap[MSG_HEARTBEAT]		= handleHeartbeat
	this.funcMap[MSG_REGISTER]		= handleRegister
	this.funcMap[MSG_UNREGISTER]	= handleUnregister
	this.funcMap[MSG_PUSH_REPLY]	= handlePushReply
	return l, nil
}

func (this *Server) Run(listener *net.TCPListener) {
	this.waitGroup.Add(1)
	defer func() {
		listener.Close()
		this.waitGroup.Done()
	}()

	//go this.dealSpamConn()
	log.Infof("comet server start\n")
	for {
		select {
		case <- this.exitCh:
			log.Infof("ask me to quit")
			return
		default:
		}

		listener.SetDeadline(time.Now().Add(2*time.Second))
		//listener.SetDeadline(time.Now().Add(this.acceptTimeout))
		//log.Infof("before accept, %d", this.acceptTimeout)
		conn, err := listener.AcceptTCP()
		//log.Infof("after accept")
		if err != nil {
			if e, ok := err.(*net.OpError); ok && e.Timeout() {
				//log.Infof("accept timeout")
				continue
			}
			log.Infof("accept failed: %v\n", err)
			continue
		}
		/*
		// first packet must sent by client in specified seconds
		if err = conn.SetReadDeadline(time.Now().Add(20)); err != nil {
			glog.Errorf("conn.SetReadDeadLine() error(%v)", err)
			conn.Close()
			continue
		}*/
		go this.handleConnection(conn)
	}
}

func (this *Server) Stop() {
	// close后，所有的exitCh都返回false
	log.Infof("stopping comet server")
	close(this.exitCh)
	this.waitGroup.Wait()
	log.Infof("comet server stopped")
}

// handle a TCP connection
func (this *Server)handleConnection(conn *net.TCPConn) {
	log.Infof("accept connection (%v)", conn)
	// handle register first
	client := waitInit(conn)
	if client == nil {
		return
	}

	for {
		/*
		select {
		case <- this.exitCh:
			log.Infof("ask me quit\n")
			return
		default:
		}
		*/

		now := time.Now()
		if now.After(client.lastActive.Add(90*time.Second)) {
			log.Infof("heartbeat timeout")
			break
		}

		//conn.SetReadDeadline(time.Now().Add(this.readTimeout))
		conn.SetReadDeadline(now.Add(10* time.Second))
		//headSize := 10
		buf := make([]byte, 10)
		n, err := io.ReadFull(conn, buf)
		if err != nil {
			if e, ok := err.(*net.OpError); ok && e.Timeout() {
				//log.Infof("read timeout, %d", n)
				continue
			}
			log.Infof("readfull failed (%v)", err)
			break
		}
		var header Header
		if err := header.Deserialize(buf[0:n]); err != nil {
			break
		}
		if header.Type != 0 {
			log.Infof("recv msg: %d, len %d", header.Type, header.Len)
		}
		data := []byte{}
		if header.Len > 0 {
			data = make([]byte, header.Len)
			if _, err := io.ReadFull(conn, data); err != nil {
				if e, ok := err.(*net.OpError); ok && e.Timeout() {
					continue
				}
				log.Infof("read from client failed: (%v)", err)
				break
			}
			log.Infof("body (%s)", data)
		}

		handler, ok := this.funcMap[header.Type]; if ok {
			ret := handler(client, &header, data)
			if ret < 0 {
				break
			}
		}
	}
	// don't use defer to improve performance
	log.Infof("close connection (%v)", conn)
	for regid, _ := range(client.regApps) {
		AMInstance.RemoveApp(regid)
	}
	CloseClient(client)
}

func waitInit(conn *net.TCPConn) (*Client) {
	conn.SetReadDeadline(time.Now().Add(10* time.Second))
	buf := make([]byte, 10)
	n, err := io.ReadFull(conn, buf)
	if err != nil {
		log.Infof("readfull header failed (%v)", err)
		conn.Close()
		return nil
	}

	var header Header
	if err := header.Deserialize(buf[0:n]); err != nil {
		log.Infof("parse header (%v)", err)
		conn.Close()
		return nil
	}

	//log.Infof("body len %d", header.Len)
	data := make([]byte, header.Len)
	if _, err := io.ReadFull(conn, data); err != nil {
		log.Infof("readfull body failed: (%v)", err)
		conn.Close()
		return nil
	}

	if header.Type != MSG_INIT {
		log.Infof("not register message, %d", header.Type)
		conn.Close()
		return nil
	}

	var msg InitMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Warnf("JSON decode failed")
		conn.Close()
		return nil
	}

	devid := msg.DeviceId
	log.Infof(">>>INIT devid (%s)", devid)
	if DevicesMap.Check(devid) {
		log.Warnf("device (%s) init in this server already", devid)
		conn.Close()
		return nil
	}

	if storage.StorageInstance.AddDevice(devid) {
		log.Warnf("storage add device (%s) failed", devid)
		conn.Close()
		return nil
	}

	client := InitClient(conn, devid)
	for _, app := range(msg.Apps) {
		AMInstance.RegisterApp(client.devId, app.AppId, app.AppKey, app.RegId)
	}
	reply := InitReplyMessage{
		Result : "0",
	}
	body, _ := json.Marshal(&reply)
	client.SendMessage(MSG_INIT_REPLY, body, nil)
	return client
}

// app注册后，才可以接收消息
func handleRegister(client *Client, header *Header, body []byte) int {
	var msg RegisterMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return -1
	}
	log.Infof(">>>REGISTER appid(%s) appkey(%s) regid(%s)", msg.AppId, msg.AppKey, msg.RegId)
	if msg.RegId != "" {
		if _, ok := client.regApps[msg.RegId]; ok {
			reply := RegisterReplyMessage{
				AppId : msg.AppId,
				RegId : msg.RegId,
				Result : 0,
			}
			b, _ := json.Marshal(reply)
			client.SendMessage(MSG_REGISTER_REPLY, b, nil)
			return 0
		}
	}
	app := AMInstance.RegisterApp(client.devId, msg.AppId, msg.AppKey, msg.RegId)
	if app == nil {
		log.Infof("AMInstance register app failed")
		reply := RegisterReplyMessage{
			AppId : msg.AppId,
			RegId : msg.RegId,
			Result : -1,
		}
		b, _ := json.Marshal(reply)
		client.SendMessage(MSG_REGISTER_REPLY, b, nil)
		return 0
	}

	client.regApps[app.RegId] = app
	reply := RegisterReplyMessage{
		AppId : msg.AppId,
		RegId : app.RegId,
		Result : 0,
	}
	b, _ := json.Marshal(reply)
	client.SendMessage(MSG_REGISTER_REPLY, b, nil)

	// handle offline messages
	msgs := storage.StorageInstance.GetOfflineMsgs(msg.AppId, app.LastMsgId)
	log.Infof("get %d offline messages: (%s) (>%d)", len(msgs), msg.AppId, app.LastMsgId)
	for _, rmsg := range(msgs) {
		msg := PushMessage{
			MsgId : rmsg.MsgId,
			AppId : rmsg.AppId,
			Type : rmsg.MsgType,
			Content : rmsg.Content,
		}
		b, _ := json.Marshal(msg)
		client.SendMessage(MSG_PUSH, b, nil)
	}
	return 0
}

func handleUnregister(client *Client, header *Header, body []byte) int {
	var msg UnregisterMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return -1
	}
	log.Infof(">>>UNREGISTER (appid %s) (appkey %s) (regid%s)", msg.AppId, msg.AppKey, msg.RegId)
	AMInstance.UnregisterApp(client.devId, msg.AppId, msg.AppKey, msg.RegId)
	result := 0
	reply := RegisterReplyMessage{
		AppId : msg.AppId,
		RegId : msg.RegId,
		Result : result,
	}
	b, _ := json.Marshal(reply)
	client.SendMessage(MSG_UNREGISTER_REPLY, b, nil)
	return 0
}

func handleHeartbeat(client *Client, header *Header, body []byte) int {
	client.lastActive = time.Now()
	return 0
}

func handlePushReply(client *Client, header *Header, body []byte) int {
	var msg PushReplyMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		log.Infof("json decode failed")
		return -1
	}

	log.Infof(">>>PUSH_REPLY (appid %s) (regid %s) (msgid %d)", msg.AppId, msg.RegId, msg.MsgId)
	// unknown regid
	app, ok := client.regApps[msg.RegId]
	if !ok {
		log.Infof("unkonw regid %s", msg.RegId)
		return 0
	}

	if msg.MsgId <= app.LastMsgId {
		log.Infof("msgid mismatch: %d <= %d", msg.MsgId, app.LastMsgId)
		return 0
	}
	if err := AMInstance.UpdateApp(msg.AppId, msg.RegId, msg.MsgId, app); err != nil {
		log.Infof("update app failed, (%s)", err)
	}
	return 0
}

