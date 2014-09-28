package comet

import (
	"log"
	"io"
	"net"
	"sync"
	"time"
	//"fmt"
	//"strings"
	"encoding/json"
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
		log.Printf("enter send routine")
		for {
			select {
			case pack := <-client.outMsgs:
				seqid := pack.client.nextSeq
				pack.msg.Header.Seq = seqid
				b, _ := pack.msg.Header.Serialize()
				conn.Write(b)
				conn.Write(pack.msg.Data)
				log.Printf("send msg ok, (%d) (%s)", pack.msg.Header.Type, pack.msg.Data)
				pack.client.nextSeq += 1
			case <-client.ctrl:
				log.Printf("leave send routine")
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
		log.Printf("failed to listen, (%v)", err)
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
	log.Printf("comet server start\n")
	for {
		select {
		case <- this.exitCh:
			log.Printf("ask me to quit")
			return
		default:
		}

		listener.SetDeadline(time.Now().Add(2*time.Second))
		//listener.SetDeadline(time.Now().Add(this.acceptTimeout))
		//log.Printf("before accept, %d", this.acceptTimeout)
		conn, err := listener.AcceptTCP()
		//log.Printf("after accept")
		if err != nil {
			if e, ok := err.(*net.OpError); ok && e.Timeout() {
				//log.Printf("accept timeout")
				continue
			}
			log.Printf("accept failed: %v\n", err)
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
	log.Printf("stopping comet server")
	close(this.exitCh)
	this.waitGroup.Wait()
	log.Printf("comet server stopped")
}

// handle a TCP connection
func (this *Server)handleConnection(conn *net.TCPConn) {
	log.Printf("accept connection (%v)", conn)
	// handle register first
	client := waitInit(conn)
	if client == nil {
		return
	}

	for {
		/*
		select {
		case <- this.exitCh:
			log.Printf("ask me quit\n")
			return
		default:
		}
		*/

		now := time.Now()
		if now.After(client.lastActive.Add(90*time.Second)) {
			log.Printf("heartbeat timeout")
			break
		}

		//conn.SetReadDeadline(time.Now().Add(this.readTimeout))
		conn.SetReadDeadline(now.Add(10* time.Second))
		//headSize := 10
		buf := make([]byte, 10)
		n, err := io.ReadFull(conn, buf)
		if err != nil {
			if e, ok := err.(*net.OpError); ok && e.Timeout() {
				//log.Printf("read timeout, %d", n)
				continue
			}
			log.Printf("readfull failed (%v)", err)
			break
		}
		var header Header
		if err := header.Deserialize(buf[0:n]); err != nil {
			break
		}
		if header.Type != 0 {
			log.Printf("recv msg: %d, len %d", header.Type, header.Len)
		}
		data := []byte{}
		if header.Len > 0 {
			data = make([]byte, header.Len)
			if _, err := io.ReadFull(conn, data); err != nil {
				if e, ok := err.(*net.OpError); ok && e.Timeout() {
					continue
				}
				log.Printf("read from client failed: (%v)", err)
				break
			}
			log.Printf("body (%s)", data)
		}

		handler, ok := this.funcMap[header.Type]; if ok {
			ret := handler(client, &header, data)
			if ret < 0 {
				break
			}
		}
	}
	// don't use defer to improve performance
	log.Printf("close connection (%v)", conn)
	for regid, _ := range(client.regApps) {
		AMInstance.RemoveApp(regid)
	}
	CloseClient(client)
}

type InitMessage struct {
	DeviceId	string	`json:"device_id"`
	Apps		[]RegisterMessage `json:"apps"`
}
type InitReplyMessage struct {
	Result	string `json:"result"`
}
func waitInit(conn *net.TCPConn) (*Client) {
	conn.SetReadDeadline(time.Now().Add(10* time.Second))
	buf := make([]byte, 10)
	n, err := io.ReadFull(conn, buf)
	if err != nil {
		log.Printf("readfull header failed (%v)", err)
		conn.Close()
		return nil
	}

	var header Header
	if err := header.Deserialize(buf[0:n]); err != nil {
		log.Printf("parse header (%v)", err)
		conn.Close()
		return nil
	}

	//log.Printf("body len %d", header.Len)
	data := make([]byte, header.Len)
	if _, err := io.ReadFull(conn, data); err != nil {
		log.Printf("readfull body failed: (%v)", err)
		conn.Close()
		return nil
	}

	if header.Type != MSG_INIT {
		log.Printf("not register message, %d", header.Type)
		conn.Close()
		return nil
	}

	var msg InitMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("JSON decode failed")
		conn.Close()
		return nil
	}

	devid := msg.DeviceId
	log.Printf("recv init devid (%s)", devid)
	if DevicesMap.Check(devid) {
		log.Printf("device (%s) init already", devid)
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

type RegisterMessage struct{
	AppId	string	`json:"app_id"`
	AppKey	string	`json:"app_key"`
	RegId	string	`json:"reg_id"`
}
type RegisterReplyMessage struct{
	AppId	string	`json:"app_id"`
	RegId	string	`json:"reg_id"`
	Result	int		`json:"result"`
}
// app注册后，才可以接收消息
func handleRegister(client *Client, header *Header, body []byte) int {
	log.Printf("handle register")
	var msg RegisterMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return -1
	}
	log.Printf("appid(%s) appkey(%s) regid(%s)", msg.AppId, msg.AppKey, msg.RegId)

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
	log.Printf("get %d offline messages for (app %s) (regid %s)", len(msgs), msg.AppId, app.RegId)
	for _, msg := range(msgs) {
		client.SendMessage(MSG_PUSH, []byte(msg), nil)
	}
	return 0
}

type UnregisterMessage struct{
	AppId	string	`json:"app_id"`
	AppKey	string	`json:"app_key"`
	RegId	string	`json:"reg_id"`
}
type UnregisterReplyMessage struct{
	AppId	string	`json:"app_id"`
	RegId	string	`json:"reg_id"`
	Result	int		`json:"result"`
}
func handleUnregister(client *Client, header *Header, body []byte) int {
	log.Printf("handle unregister")
	var msg UnregisterMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return -1
	}
	log.Printf("(%s) (%s) (%s)", msg.AppId, msg.AppKey, msg.RegId)
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

type PushMessage struct {
	MsgId		int64	`json:"msg_id"`
	AppId		string	`json:"app_id"`
	MsgType		int		`json:"msg_type"`
	Payload		string	`json:"payload"`
}
type PushReplyMessage struct {
	MsgId	int64	`json:"msg_id"`
	AppId	string	`json:"app_id"`
	RegId	string	`json:"reg_id"`
}
func handlePushReply(client *Client, header *Header, body []byte) int {
	var msg PushReplyMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return -1
	}

	// unknown regid
	app, ok := client.regApps[msg.RegId]
	if !ok {
		return 0
	}

	if msg.MsgId <= app.LastMsgId {
		log.Printf("msgid mismatch: %d <= %d", msg.MsgId, app.LastMsgId)
		return 0
	}
	if err := AMInstance.UpdateApp(msg.AppId, msg.RegId, msg.MsgId, app); err != nil {
		log.Printf("update app failed, (%s)", err)
	}
	return 0
}

