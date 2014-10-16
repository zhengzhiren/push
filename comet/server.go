package comet

import (
	//"fmt"
	"io"
	"net"
	"sync"
	"time"
	//"strings"
	"encoding/json"
	"github.com/chenyf/push/auth"
	"github.com/chenyf/push/storage"
	"github.com/chenyf/push/utils/safemap"
	log "github.com/cihub/seelog"
	//"github.com/bitly/go-simplejson"
)

const (
	MAX_BODY_LEN = 1024
)

type MsgHandler func(*net.TCPConn, *Client, *Header, []byte) int

type Pack struct {
	msg    *Message
	client *Client
	reply  chan *Message
}

type Client struct {
	devId      string
	regApps    map[string]*App
	outMsgs    chan *Pack
	nextSeq    uint32
	lastActive time.Time
	ctrl       chan bool
}

type Server struct {
	exitCh        chan bool
	waitGroup     *sync.WaitGroup
	funcMap       map[uint8]MsgHandler
	acceptTimeout time.Duration
	readTimeout   time.Duration
	writeTimeout  time.Duration
	maxMsgLen     uint32
}

func NewServer() *Server {
	return &Server{
		exitCh:        make(chan bool),
		waitGroup:     &sync.WaitGroup{},
		funcMap:       make(map[uint8]MsgHandler),
		acceptTimeout: 60,
		readTimeout:   60,
		writeTimeout:  60,
		maxMsgLen:     2048,
	}
}

func (client *Client) SendMessage(msgType uint8, body []byte, reply chan *Message) {
	header := Header{
		Type: msgType,
		Ver:  0,
		Seq:  0,
		Len:  uint32(len(body)),
	}
	msg := &Message{
		Header: header,
		Data:   body,
	}

	pack := &Pack{
		msg:    msg,
		client: client,
		reply:  reply,
	}
	client.outMsgs <- pack
}

var (
	DevicesMap *safemap.SafeMap = safemap.NewSafeMap()
)

func InitClient(conn *net.TCPConn, devid string) *Client {
	client := &Client{
		devId:      devid,
		regApps:    make(map[string]*App),
		nextSeq:    1,
		lastActive: time.Now(),
		outMsgs:    make(chan *Pack, 100),
		ctrl:       make(chan bool),
	}
	DevicesMap.Set(devid, client)

	go func() {
		log.Infof("%p: enter send routine", conn)
		for {
			select {
			case pack := <-client.outMsgs:
				seqid := pack.client.nextSeq
				pack.msg.Header.Seq = seqid
				b, _ := pack.msg.Header.Serialize()
				conn.Write(b)
				conn.Write(pack.msg.Data)
				log.Infof("%p: send msg: (%d) (%s)", conn, pack.msg.Header.Type, pack.msg.Data)
				pack.client.nextSeq += 1
				time.Sleep(1 * time.Second)
			case <-client.ctrl:
				log.Infof("%p: leave send routine", conn)
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
		log.Errorf("failed to listen, (%v)", err)
		return nil, err
	}
	this.funcMap[MSG_HEARTBEAT]  = handleHeartbeat
	this.funcMap[MSG_REGISTER]   = handleRegister
	this.funcMap[MSG_UNREGISTER] = handleUnregister
	this.funcMap[MSG_PUSH_REPLY] = handlePushReply
	return l, nil
}

func (this *Server) Run(listener *net.TCPListener) {
	this.waitGroup.Add(1)
	defer func() {
		listener.Close()
		this.waitGroup.Done()
	}()

	//go this.dealSpamConn()
	log.Debugf("comet server start\n")
	for {
		select {
		case <-this.exitCh:
			log.Debugf("ask me to quit")
			return
		default:
		}

		listener.SetDeadline(time.Now().Add(2 * time.Second))
		//listener.SetDeadline(time.Now().Add(this.acceptTimeout))
		//log.Debugf("before accept, %d", this.acceptTimeout)
		conn, err := listener.AcceptTCP()
		//log.Debugf("after accept")
		if err != nil {
			if e, ok := err.(*net.OpError); ok && e.Timeout() {
				//log.Debugf("accept timeout")
				continue
			}
			log.Debugf("accept failed: %v\n", err)
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
	log.Debugf("stopping comet server")
	close(this.exitCh)
	this.waitGroup.Wait()
	log.Debugf("comet server stopped")
}

// handle a TCP connection
func (this *Server) handleConnection(conn *net.TCPConn) {
	log.Debugf("accept connection from (%s) (%p)", conn.RemoteAddr(), conn)
	// handle register first
	client := waitInit(conn)
	if client == nil {
		return
	}

	var (
		readHeader = true
		bytesRead  = 0
		data       []byte
		header     Header
		startTime  time.Time
	)

	for {
		select {
		case <-this.exitCh:
			log.Debugf("ask me quit\n")
			goto out
		default:
		}

		now := time.Now()
		if now.After(client.lastActive.Add(90 * time.Second)) {
			log.Debugf("%p: heartbeat timeout", conn)
			break
		}

		//conn.SetReadDeadline(time.Now().Add(this.readTimeout))
		conn.SetReadDeadline(now.Add(10 * time.Second))

		if readHeader {
			buf := make([]byte, HEADER_SIZE)
			n, err := io.ReadFull(conn, buf)
			if err != nil {
				if e, ok := err.(*net.OpError); ok && e.Timeout() {
					//log.Debugf("read timeout, %d", n)
					continue
				}
				log.Debugf("%p: readfull failed (%v)", conn, err)
				break
			}
			if err := header.Deserialize(buf[0:n]); err != nil {
				break
			}

			if header.Len > MAX_BODY_LEN {
				log.Warnf("%p: header len too big %d", conn, header.Len)
				break
			}
			/*
				if header.Type != 0 {
					log.Debugf("recv msg: %d, len %d", header.Type, header.Len)
				}*/
			if header.Len > 0 {
				data = make([]byte, header.Len)
				readHeader = false
				bytesRead = 0
				startTime = time.Now()
				continue
			}
		} else {
			n, err := conn.Read(data[bytesRead:])
			if err != nil {
				if e, ok := err.(*net.OpError); ok && e.Timeout() {
					if now.After(startTime.Add(60 * time.Second)) {
						log.Debugf("%p: read packet data timeout", conn)
						goto out
					}
				} else {
					log.Debugf("%p: read from client failed: (%v)", conn, err)
					goto out
				}
			}
			if n > 0 {
				bytesRead += n
			}
			if uint32(bytesRead) < header.Len {
				continue
			}
			readHeader = true
			log.Debugf("%p: body (%s)", conn, data)
		}

		handler, ok := this.funcMap[header.Type]
		if ok {
			ret := handler(conn, client, &header, data)
			if ret < 0 {
				break
			}
		}
	}

out:
	// don't use defer to improve performance
	log.Debugf("%p: close connection", conn)
	for regid, _ := range client.regApps {
		AMInstance.RemoveApp(regid)
	}
	CloseClient(client)
}

func waitInit(conn *net.TCPConn) *Client {
	// 要求客户端尽快发送初始化消息
	conn.SetReadDeadline(time.Now().Add(20 * time.Second))
	buf := make([]byte, HEADER_SIZE)
	n, err := io.ReadFull(conn, buf)
	if err != nil {
		log.Debugf("%p: readfull header failed (%v)", conn, err)
		conn.Close()
		return nil
	}

	var header Header
	if err := header.Deserialize(buf[0:n]); err != nil {
		log.Warnf("%p: parse header failed: (%v)", conn, err)
		conn.Close()
		return nil
	}

	if header.Len > MAX_BODY_LEN {
		log.Warnf("%p: header len too big: %d", conn, header.Len)
		conn.Close()
		return nil
	}
	data := make([]byte, header.Len)
	if _, err := io.ReadFull(conn, data); err != nil {
		log.Debugf("%p: readfull body failed: (%v)", conn, err)
		conn.Close()
		return nil
	}

	if header.Type != MSG_INIT {
		log.Warnf("%p: not register message, %d", conn, header.Type)
		conn.Close()
		return nil
	}

	var msg InitMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Warnf("%p: JSON decode failed: (%v)", conn, err)
		conn.Close()
		return nil
	}

	devid := msg.DeviceId
	if devid == "" {
		log.Warnf("%p: invalid device id", conn)
		conn.Close()
		return nil
	}

	log.Debugf("%p: INIT devid (%s)", conn, devid)
	if DevicesMap.Check(devid) {
		log.Warnf("%p: device (%s) init in this server already", conn, devid)
		conn.Close()
		return nil
	}

	client := InitClient(conn, devid)
	reply := InitReplyMessage{
		Result: 0,
	}

	if msg.Sync == 0 {
		for _, info := range(msg.Apps) {
			app := AMInstance.RegisterApp(client.devId, info.RegId, info.AppId, "")
			if app != nil {
				client.regApps[info.RegId] = app
			}
		}
	} else {
		// 客户端要求同步服务端的数据
		// 看存储系统中是否有此设备的数据
		infos := AMInstance.LoadAppInfosByDevice(devid)
		for regid, info := range infos {
			log.Debugf("load app (%s) (%s) of device (%s)", info.AppId, regid, devid)
			app := AMInstance.AddApp(client.devId, regid, info)
			client.regApps[regid] = app
			reply.Apps = append(reply.Apps, Base2{info.AppId, regid, ""})
		}
	}

	// 先发送响应消息
	body, _ := json.Marshal(&reply)
	client.SendMessage(MSG_INIT_REPLY, body, nil)

	// 处理离线消息
	for _, app := range(client.regApps) {
		msgs := storage.Instance.GetOfflineMsgs(app.AppId, app.LastMsgId)
		log.Debugf("%p: get %d offline messages: (%s) (>%d)", conn, len(msgs), app.AppId, app.LastMsgId)
		for _, rmsg := range msgs {
			msg := PushMessage{
				MsgId:   rmsg.MsgId,
				AppId:   rmsg.AppId,
				Type:    rmsg.MsgType,
				Content: rmsg.Content,
			}
			b, _ := json.Marshal(msg)
			client.SendMessage(MSG_PUSH, b, nil)
		}
	}
	return client
}

// app注册后，才可以接收消息
func handleRegister(conn *net.TCPConn, client *Client, header *Header, body []byte) int {
	var msg RegisterMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		log.Warnf("%p: json decode failed: (%v)", conn, err)
		return -1
	}
	log.Debugf("%p: REGISTER appid(%s) appkey(%s) regid(%s) token(%s)", conn, msg.AppId, msg.AppKey, msg.RegId, msg.Token)

	var uid string = ""
	var ok bool
	if msg.Token != "" {
		ok, uid = auth.Instance.Auth(msg.Token)
		if !ok {
			log.Warnf("%p: auth failed", conn)
			return -1
		}
	}
	regid := RegId(client.devId, msg.AppId, uid)
	log.Debugf("%p: uid (%s), regid (%s)", conn, uid, regid)
	if _, ok := client.regApps[regid]; ok {
		// 已经在内存中，直接返回
		reply := RegisterReplyMessage{
			AppId:  msg.AppId,
			RegId:  regid,
			Result: 0,
		}
		b, _ := json.Marshal(reply)
		client.SendMessage(MSG_REGISTER_REPLY, b, nil)
		return 0
	}

	// 到app管理中心去注册
	app := AMInstance.RegisterApp(client.devId, regid, msg.AppId, uid)
	if app == nil {
		log.Warnf("%p: AMInstance register app failed", conn)
		reply := RegisterReplyMessage{
			AppId:  msg.AppId,
			RegId:  msg.RegId,
			Result: -1,
		}
		b, _ := json.Marshal(reply)
		client.SendMessage(MSG_REGISTER_REPLY, b, nil)
		return 0
	}
	// 记录到client管理的hash table中
	client.regApps[regid] = app
	reply := RegisterReplyMessage{
		AppId:  msg.AppId,
		RegId:  regid,
		Result: 0,
	}
	b, _ := json.Marshal(reply)
	client.SendMessage(MSG_REGISTER_REPLY, b, nil)

	// 处理离线消息
	msgs := storage.Instance.GetOfflineMsgs(msg.AppId, app.LastMsgId)
	log.Debugf("%p: get %d offline messages: (%s) (>%d)", conn, len(msgs), msg.AppId, app.LastMsgId)
	for _, rmsg := range msgs {
		msg := PushMessage{
			MsgId:   rmsg.MsgId,
			AppId:   rmsg.AppId,
			Type:    rmsg.MsgType,
			Content: rmsg.Content,
		}
		b, _ := json.Marshal(msg)
		client.SendMessage(MSG_PUSH, b, nil)
	}
	return 0
}

func handleUnregister(conn *net.TCPConn, client *Client, header *Header, body []byte) int {
	var msg UnregisterMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		log.Warnf("%p: json decode failed: (%v)", conn, err)
		return -1
	}
	log.Debugf("%p: UNREGISTER (appid %s) (appkey %s) (regid%s)", conn, msg.AppId, msg.AppKey, msg.RegId)

	var uid string = ""
	var ok bool
	if msg.Token != "" {
		ok, uid = auth.Instance.Auth(msg.Token)
		if !ok {
			log.Warnf("%p: auth failed", conn)
			return -1
		}
	}
	log.Debugf("%p: uid is (%s)", conn, uid)
	AMInstance.UnregisterApp(client.devId, msg.RegId, msg.AppId, uid)
	result := 0
	reply := RegisterReplyMessage{
		AppId:  msg.AppId,
		RegId:  msg.RegId,
		Result: result,
	}
	b, _ := json.Marshal(reply)
	client.SendMessage(MSG_UNREGISTER_REPLY, b, nil)
	return 0
}

func handleHeartbeat(conn *net.TCPConn, client *Client, header *Header, body []byte) int {
	client.lastActive = time.Now()
	return 0
}

func handlePushReply(conn *net.TCPConn, client *Client, header *Header, body []byte) int {
	var msg PushReplyMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		log.Warnf("%p: json decode failed: (%v)", conn, err)
		return -1
	}

	log.Debugf("%p: PUSH_REPLY (appid %s) (regid %s) (msgid %d)", conn, msg.AppId, msg.RegId, msg.MsgId)
	// unknown regid
	app, ok := client.regApps[msg.RegId]
	if !ok {
		log.Warnf("%p: unkonw regid %s", conn, msg.RegId)
		return 0
	}

	if msg.MsgId <= app.LastMsgId {
		log.Warnf("%p: msgid mismatch: %d <= %d", conn, msg.MsgId, app.LastMsgId)
		return 0
	}
	if err := AMInstance.UpdateApp(msg.AppId, msg.RegId, msg.MsgId, app); err != nil {
		log.Warnf("%p: update app failed, (%s)", conn, err)
	}
	return 0
}
