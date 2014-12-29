package comet

import (
	"encoding/json"
	"io"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenyf/push/storage"
	"github.com/chenyf/push/utils"
	"github.com/chenyf/push/utils/safemap"
	log "github.com/cihub/seelog"
)

type MsgHandler func(*net.TCPConn, *Client, *Header, []byte) int

type Pack struct {
	msg    *Message
	client *Client
	reply  chan *Message
}

type Client struct {
	devId           string
	RegApps         map[string]*RegApp
	outMsgs         chan *Pack
	nextSeq         uint32
	lastActive      time.Time
	WaitingChannels map[uint32]chan *Message
	ctrl            chan bool // notify sendout routing to quit when close connection
	Broken          bool
}

type Server struct {
	Name          string // unique name of this server
	exitCh        chan bool
	wg            *sync.WaitGroup
	funcMap       map[uint8]MsgHandler
	blackDevices  []string
	acceptTimeout time.Duration
	readTimeout   time.Duration
	writeTimeout  time.Duration
	hbTimeout     time.Duration
	maxBodyLen    uint32
	maxClients    uint32
	clientCount   uint32
}

func NewServer(ato uint32, rto uint32, wto uint32, hto uint32, maxBodyLen uint32, maxClients uint32) *Server {
	return &Server{
		Name:          utils.GetLocalIP(),
		exitCh:        make(chan bool),
		wg:            &sync.WaitGroup{},
		funcMap:       make(map[uint8]MsgHandler),
		acceptTimeout: time.Duration(ato),
		readTimeout:   time.Duration(rto),
		writeTimeout:  time.Duration(wto),
		hbTimeout:     time.Duration(hto),
		maxBodyLen:    maxBodyLen,
		maxClients:    maxClients,
		clientCount:   0,
	}
}

func (client *Client) SendMessage(msgType uint8, seq uint32, body []byte, reply chan *Message) (uint32, bool) {
	if reply != nil && len(client.WaitingChannels) == 10 {
		return 0, false
	}
	if seq == 0 {
		seq = client.NextSeq()
	}
	bodylen := 0
	if body != nil {
		bodylen = len(body)
	}
	header := Header{
		Type: msgType,
		Ver:  0,
		Seq:  seq,
		Len:  uint32(bodylen),
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
	return seq, true
}

var (
	DevicesMap *safemap.SafeMap = safemap.NewSafeMap()
)

func (this *Client) MsgTimeout(seq uint32) {
	delete(this.WaitingChannels, seq)
}

func (this *Client) NextSeq() uint32 {
	return atomic.AddUint32(&this.nextSeq, 1)
}

func (this *Server) InitClient(conn *net.TCPConn, devid string) *Client {
	// save the client device Id to storage
	if err := storage.Instance.AddDevice(this.Name, devid); err != nil {
		log.Warnf("failed to put device %s into redis:", devid, err)
		return nil
	}

	client := &Client{
		devId:           devid,
		RegApps:         make(map[string]*RegApp),
		nextSeq:         100, //sequence number begin from 100 each time
		lastActive:      time.Now(),
		outMsgs:         make(chan *Pack, 100),
		WaitingChannels: make(map[uint32]chan *Message, 10), //TODO
		ctrl:            make(chan bool),
		Broken:          false,
	}
	DevicesMap.Set(devid, client)

	go func() {
		//log.Infof("%p: enter send routine", conn)
		for {
			select {
			case pack := <-client.outMsgs:
				b, _ := pack.msg.Header.Serialize()
				if _, err := conn.Write(b); err != nil {
					log.Infof("sendout header failed, %s", err)
					client.Broken = true
					return
				}
				if pack.msg.Data != nil {
					if _, err := conn.Write(pack.msg.Data); err != nil {
						log.Infof("sendout body failed, %s", err)
						client.Broken = true
						return
					}
				}
				// add reply channel
				if pack.reply != nil {
					client.WaitingChannels[pack.msg.Header.Seq] = pack.reply
				}
				log.Infof("%s: send msg: (%d) (%d) (%s)",
					client.devId,
					pack.msg.Header.Type,
					pack.msg.Header.Seq,
					pack.msg.Data)
				time.Sleep(10 * time.Millisecond)
			//case seq := <-client.seqCh:
			//	delete(WaitingChannels, seq)
			case <-client.ctrl:
				//log.Infof("%p: leave send routine", conn)
				return
			}
		}
	}()
	return client
}

func (this *Server) CloseClient(client *Client) {
	client.ctrl <- true
	if err := storage.Instance.RemoveDevice(this.Name, client.devId); err != nil {
		log.Errorf("failed to remove device %s from redis:", client.devId, err)
	}
	DevicesMap.Delete(client.devId)
}

func (this *Server) Init(addr string) (*net.TCPListener, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp4", addr)
	l, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		log.Errorf("failed to listen, (%v)", err)
		return nil, err
	}
	this.funcMap[MSG_HEARTBEAT] = handleHeartbeat
	this.funcMap[MSG_REGISTER] = handleRegister
	this.funcMap[MSG_UNREGISTER] = handleUnregister
	this.funcMap[MSG_PUSH_REPLY] = handlePushReply
	this.funcMap[MSG_SUBSCRIBE] = handleSubscribe
	this.funcMap[MSG_UNSUBSCRIBE] = handleUnsubscribe
	this.funcMap[MSG_GET_TOPICS] = handleGetTopics
	this.funcMap[MSG_CMD_REPLY] = handleCmdReply
	this.funcMap[MSG_REGISTER2] = handleRegister2
	this.funcMap[MSG_UNREGISTER2] = handleUnregister2

	if err := storage.Instance.InitDevices(this.Name); err != nil {
		log.Errorf("failed to InitDevices: %s", err.Error())
		return nil, err
	}

	if this.blackDevices, err = storage.Instance.SetMembers("db_black_devices"); err != nil {
		sort.Strings(this.blackDevices)
	}

	// keep the data of this node not expired on redis
	go func() {
		for {
			select {
			case <-this.exitCh:
				log.Infof("Exiting storage refreshing routine")
				return
			case <-time.After(10 * time.Second):
				storage.Instance.RefreshDevices(this.Name, 30)
			}
		}
	}()

	return l, nil
}

func (this *Server) Run(listener *net.TCPListener) {
	defer func() {
		listener.Close()
	}()

	//go this.dealSpamConn()
	log.Infof("Starting comet server on: %s", listener.Addr().String())

	for {
		select {
		case <-this.exitCh:
			log.Debugf("ask me to quit")
			return
		default:
		}

		listener.SetDeadline(time.Now().Add(2 * time.Second))
		//listener.SetDeadline(time.Now().Add(this.acceptTimeout))
		conn, err := listener.AcceptTCP()
		if err != nil {
			if e, ok := err.(*net.OpError); ok && e.Timeout() {
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
	this.wg.Wait()
	log.Debugf("comet server stopped")
}

func myread(conn *net.TCPConn, buf []byte) int {
	n, err := io.ReadFull(conn, buf)
	if err != nil {
		if e, ok := err.(*net.OpError); ok && e.Timeout() {
			return n
		}
		log.Debugf("%p: readfull failed (%v)", conn, err)
		return -1
	}
	return n
}

// handle a TCP connection
func (this *Server) handleConnection(conn *net.TCPConn) {
	log.Debugf("New connection (%p) from (%s)", conn, conn.RemoteAddr())
	// handle register first
	if this.clientCount >= 10000 {
		log.Warnf("too more client, refuse")
		conn.Close()
		return
	}
	client := waitInit(this, conn)
	if client == nil {
		conn.Close()
		return
	}
	this.clientCount++

	var (
		readflag         = 0
		nRead     int    = 0
		n         int    = 0
		headBuf   []byte = make([]byte, HEADER_SIZE)
		dataBuf   []byte
		header    Header
		startTime time.Time
	)

	for {
		select {
		case <-this.exitCh:
			log.Debugf("%s: ask me quit", client.devId)
			break
		default:
		}
		if client.Broken {
			log.Debugf("%s: client broken", client.devId)
			break
		}

		now := time.Now()
		if now.After(client.lastActive.Add(this.hbTimeout * time.Second)) {
			log.Debugf("%s: heartbeat timeout", client.devId)
			//client.SendMessage(MSG_CHECK, 0, nil, nil)
			break
		}

		//conn.SetReadDeadline(time.Now().Add(this.readTimeout))
		conn.SetReadDeadline(now.Add(5 * time.Second))
		if readflag == 0 {
			// read first byte
			n := myread(conn, headBuf[0:1])
			if n < 0 {
				break
			} else if n == 0 {
				continue
			}
			if uint8(headBuf[0]) == uint8(0) {
				handleHeartbeat(conn, client, nil, nil)
				continue
			}
			nRead += n
			readflag = 1
		}
		if readflag == 1 {
			// read header
			n = myread(conn, headBuf[nRead:])
			if n < 0 {
				break
			} else if n == 0 {
				continue
			}
			nRead += n
			if uint32(nRead) < HEADER_SIZE {
				continue
			}
			if err := header.Deserialize(headBuf[0:HEADER_SIZE]); err != nil {
				log.Warnf("%s: header deserialize failed", client.devId)
				break
			}

			if header.Len > this.maxBodyLen {
				log.Warnf("%s: header len too big %d", client.devId, header.Len)
				break
			}
			if header.Len > 0 {
				dataBuf = make([]byte, header.Len)
				readflag = 2
				nRead = 0
				startTime = time.Now()
			} else {
				readflag = 0
				nRead = 0
				continue
			}
		}
		if readflag == 2 {
			// read body
			n = myread(conn, dataBuf[nRead:])
			if n < 0 {
				break
			} else if n == 0 {
				continue
			}
			nRead += n
			if uint32(nRead) < header.Len {
				if now.After(startTime.Add(60 * time.Second)) {
					log.Infof("%s: read body timeout", client.devId)
					break
				}
				continue
			}
			//log.Debugf("%s: body (%s)", client.devId, dataBuf)
		}

		if handler, ok := this.funcMap[header.Type]; ok {
			client.lastActive = time.Now()
			if ret := handler(conn, client, &header, dataBuf); ret < 0 {
				break
			}
		} else {
			log.Warnf("%s: unknown message type %d", client.devId, header.Type)
		}
		readflag = 0
		nRead = 0
	}

	// don't use defer to improve performance
	log.Debugf("%s: close connection", client.devId)
	for _, regapp := range client.RegApps {
		AMInstance.DelApp(regapp.RegId)
	}
	this.CloseClient(client)
	conn.Close()
	this.clientCount--
}

func handleOfflineMsgs(client *Client, regapp *RegApp) {
	msgs := storage.Instance.GetOfflineMsgs(regapp.AppId, regapp.RegId, regapp.RegTime, regapp.LastMsgId)
	if len(msgs) == 0 {
		return
	}
	log.Debugf("%s: get %d offline messages: (%s) (>%d)", regapp.DevId, len(msgs), regapp.AppId, regapp.LastMsgId)
	for _, rawMsg := range msgs {
		ok := false
		switch rawMsg.PushType {
		case PUSH_TYPE_ALL:
			ok = true
		case PUSH_TYPE_REGID:
			for _, regid := range rawMsg.PushParams.RegId {
				if regapp.RegId == regid {
					ok = true
					break
				}
			}
		case PUSH_TYPE_USERID:
			if regapp.UserId == "" {
				continue
			}
			for _, uid := range rawMsg.PushParams.UserId {
				if regapp.UserId == uid {
					ok = true
					break
				}
			}
		case PUSH_TYPE_DEVID:
			for _, devid := range rawMsg.PushParams.DevId {
				if client.devId == devid {
					ok = true
					break
				}
			}
		case PUSH_TYPE_TOPIC:
			for _, topic := range regapp.Topics {
				if topic == rawMsg.PushParams.Topic {
					ok = true
					break
				}
			}
		default:
			continue
		}

		if ok {
			msg := PushMessage{
				MsgId: rawMsg.MsgId,
				AppId: rawMsg.AppId,
				Type:  rawMsg.MsgType,
			}
			if rawMsg.MsgType == MSG_TYPE_MESSAGE {
				msg.Content = rawMsg.Content
			} else {
				b, _ := json.Marshal(rawMsg.Notification)
				msg.Content = string(b)
			}
			b, _ := json.Marshal(msg)
			client.SendMessage(MSG_PUSH, 0, b, nil)
		}
	}
}

func inBlacklist(server *Server, devId string) bool {
	for _, s := range server.blackDevices {
		if s == devId {
			return true
		}
	}
	return false
}

func checkDevIdFormat(devId string) bool {
	if devId == "" {
		return false
	}
	return true
}

func waitInit(server *Server, conn *net.TCPConn) *Client {
	// 要求客户端尽快发送初始化消息
	conn.SetReadDeadline(time.Now().Add(20 * time.Second))
	buf := make([]byte, HEADER_SIZE)
	if n := myread(conn, buf); n <= 0 {
		conn.Close()
		return nil
	}

	var header Header
	if err := header.Deserialize(buf[0:HEADER_SIZE]); err != nil {
		log.Warnf("%p: parse header failed: (%v)", conn, err)
		conn.Close()
		return nil
	}

	if header.Type != MSG_INIT {
		log.Warnf("%p: not INIT message, %d", conn, header.Type)
		conn.Close()
		return nil
	}

	if header.Len > server.maxBodyLen {
		log.Warnf("%p: header len too big: %d", conn, header.Len)
		conn.Close()
		return nil
	}
	data := make([]byte, header.Len)
	if n := myread(conn, data); n <= 0 {
		conn.Close()
		return nil
	}

	var request InitMessage
	if err := json.Unmarshal(data, &request); err != nil {
		log.Warnf("%p: decode INIT body failed: (%v)", conn, err)
		conn.Close()
		return nil
	}

	devid := request.DevId
	if !checkDevIdFormat(devid) {
		log.Warnf("%p: invalid device id (%s)", conn, devid)
		conn.Close()
		return nil
	}
	if inBlacklist(server, devid) {
		conn.Close()
		return nil
	}
	log.Debugf("%p: INIT seq (%d) body(%s)", conn, header.Seq, data)

	for {
		x := DevicesMap.Get(devid)
		if x != nil {
			client := x.(*Client)
			client.Broken = true
			time.Sleep(1 * time.Second)
			log.Infof("%s (%p): wait old connection close", devid, conn)
		} else {
			break
		}
	}
	/*if DevicesMap.Check(devid) {
		log.Infof("%p: device (%s) init in this server already", conn, devid)
		conn.Close()
		return nil
	}*/

	// check if the device Id has connected to other servers
	serverName, err := storage.Instance.CheckDevice(devid)
	if err != nil {
		log.Errorf("%s: failed to check device existence: (%s)", devid, err)
		conn.Close()
		return nil
	}
	if serverName != "" {
		log.Warnf("%s: has connected with server (%s)", devid, serverName)
		conn.Close()
		return nil
	}

	client := server.InitClient(conn, devid)
	if client == nil {
		conn.Close()
		return nil
	}
	reply := InitReplyMessage{
		Result: 0,
		HB:     30,
		Reconn: 60,
	}

	// TODO: below code should check carefully
	if request.Sync == 0 {
		// client give me regapp infos
		for _, info := range request.Apps {
			if info.AppId == "" || info.RegId == "" {
				log.Warnf("appid or regid is empty")
				continue
			}
			// these regapps should in storage already
			regapp := AMInstance.RegisterApp2(client.devId, info.RegId, info.AppId, "")
			if regapp != nil {
				client.RegApps[info.AppId] = regapp
			}
		}
	} else {
		// 客户端要求同步服务端的数据
		// 看存储系统中是否有此设备的数据
		reply.Apps = []Base2{}
		infos := AMInstance.LoadAppInfosByDevice(devid)
		for regid, info := range infos {
			log.Debugf("%s: load app (%s) (%s)", devid, info.AppId, regid)
			b, err := storage.Instance.HashGet("db_apps", info.AppId)
			if err != nil || b == nil {
				continue
			}
			var rawapp storage.RawApp
			if err := json.Unmarshal(b, &rawapp); err != nil {
				continue
			}
			// add into memory
			regapp := AMInstance.AddApp(client.devId, regid, info)
			client.RegApps[info.AppId] = regapp
			reply.Apps = append(reply.Apps, Base2{AppId: info.AppId, RegId: regid, Pkg: rawapp.Pkg})
		}
	}

	// 先发送响应消息
	body, _ := json.Marshal(&reply)
	client.SendMessage(MSG_INIT_REPLY, header.Seq, body, nil)

	// 处理离线消息
	for _, regapp := range client.RegApps {
		handleOfflineMsgs(client, regapp)
	}
	return client
}
