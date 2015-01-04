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
	devId        string
	RegApps      map[string]*RegApp
	outMsgs      chan *Pack
	nextSeq      uint32
	lastActive   time.Time
	waitChannels map[uint32]chan *Message
	ctrl         chan bool // notify sendout routing to quit when close connection
	broken       bool
}

type Server struct {
	Name          string // unique name of this server
	exitCh        chan bool
	wg            *sync.WaitGroup
	funcMap       map[uint8]MsgHandler
	blackDevices  []string
	blackUpdate   time.Time
	acceptTimeout time.Duration
	readTimeout   time.Duration
	writeTimeout  time.Duration
	hbTimeout     time.Duration
	maxBodyLen    uint32
	maxClients    uint32
	clientCount   uint32
}

var (
	msgNames = map[uint8]string{
		MSG_INIT_REPLY:        "InitReply",
		MSG_REGISTER_REPLY:    "RegisterReply",
		MSG_UNREGISTER_REPLY:  "UnregisterReply",
		MSG_PUSH:              "Push",
		MSG_SUBSCRIBE_REPLY:   "SubscribeReply",
		MSG_UNSUBSCRIBE_REPLY: "UnsubscribeReply",
		MSG_GET_TOPICS_REPLY:  "GetTopicsReply",
		MSG_CMD:               "Command",
		MSG_REGISTER2_REPLY:   "Register2Reply",
		MSG_UNREGISTER2_REPLY: "Unregister2Reply",
	}

	DevicesMap *safemap.SafeMap = safemap.NewSafeMap()
)

func myRead(conn *net.TCPConn, buf []byte) int {
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

func (client *Client) SendMessage(msgType uint8, seq uint32, body []byte, reply chan *Message) (uint32, bool) {
	if reply != nil && len(client.waitChannels) == 10 {
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

func (this *Client) MsgTimeout(seq uint32) {
	delete(this.waitChannels, seq)
}

func (this *Client) NextSeq() uint32 {
	return atomic.AddUint32(&this.nextSeq, 1)
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

func (this *Server) Init(addr string) (*net.TCPListener, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp4", addr)
	l, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		log.Errorf("failed to listen, (%v)", err)
		return nil, err
	}
	log.Infof("start comet server at (%s)", addr)
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
	this.funcMap[MSG_STATS] = handleStats

	if err := storage.Instance.InitDevices(this.Name); err != nil {
		log.Errorf("failed to InitDevices: %s", err.Error())
		return nil, err
	}

	if this.blackDevices, err = storage.Instance.SetMembers("db_black_devices"); err != nil {
		sort.Strings(this.blackDevices)
		this.blackUpdate = time.Now()
	}

	return l, nil
}

func (this *Server) Run(listener *net.TCPListener) {
	defer func() {
		listener.Close()
	}()

	//go this.dealSpamConn()
	log.Infof("Starting comet server on: %s", listener.Addr().String())

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

	for {
		select {
		case <-this.exitCh:
			log.Debugf("ask me to quit")
			return
		default:
		}

		listener.SetDeadline(time.Now().Add(3 * time.Second))
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

func (this *Server) createClient(conn *net.TCPConn, devid string) *Client {
	// save the client device Id to storage
	if err := storage.Instance.AddDevice(this.Name, devid); err != nil {
		log.Warnf("failed to put device %s into redis:", devid, err)
		return nil
	}
	client := &Client{
		devId:        devid,
		RegApps:      make(map[string]*RegApp),
		nextSeq:      100, //sequence number begin from 100 each time
		lastActive:   time.Now(),
		outMsgs:      make(chan *Pack, 100),
		waitChannels: make(map[uint32]chan *Message, 10), //TODO
		ctrl:         make(chan bool),
		broken:       false,
	}
	DevicesMap.Set(devid, client)

	go func() {
		//log.Infof("%p: enter send routine", conn)
		for {
			select {
			case pack := <-client.outMsgs:
				b, _ := pack.msg.Header.Serialize()
				if _, err := conn.Write(b); err != nil {
					log.Infof("%s %p:sendout header failed, %s", devid, conn, err)
					client.broken = true
					return
				}
				if pack.msg.Data != nil {
					if _, err := conn.Write(pack.msg.Data); err != nil {
						log.Infof("%s %p:sendout body failed, %s", devid, conn, err)
						client.broken = true
						return
					}
				}
				// add reply channel
				if pack.reply != nil {
					client.waitChannels[pack.msg.Header.Seq] = pack.reply
				}
				msgname, ok := msgNames[pack.msg.Header.Type]
				if !ok {
					msgname = "Unknown"
				}
				log.Infof("%s %p: SEND %s (%s) type(%d) seq(%d)",
					client.devId,
					conn,
					msgname,
					pack.msg.Data,
					pack.msg.Header.Type,
					pack.msg.Header.Seq)
				time.Sleep(10 * time.Millisecond)
			case <-client.ctrl:
				log.Infof("%s %p: leave send routine", devid, conn)
				return
			}
		}
	}()
	return client
}

func (this *Server) closeClient(client *Client) {
	client.ctrl <- true
	if err := storage.Instance.RemoveDevice(this.Name, client.devId); err != nil {
		log.Warnf("failed to remove device %s from redis:", client.devId, err)
	}
	DevicesMap.Delete(client.devId)
}

// handle a TCP connection
func (this *Server) handleConnection(conn *net.TCPConn) {
	log.Debugf("new connection (%p) from (%s)", conn, conn.RemoteAddr())
	// handle register first
	if this.clientCount >= this.maxClients {
		log.Warnf("too more client, refuse")
		conn.Close()
		return
	}
	client := this.waitInit(conn)
	if client == nil {
		conn.Close()
		return
	}
	this.clientCount++

	var (
		readStep  int    = 0 //0: first byte, 1: header, 2:body
		nRead     int    = 0
		headBuf   []byte = make([]byte, HEADER_SIZE)
		dataBuf   []byte
		header    Header
		startTime time.Time
	)

	// main routine: read message from client
	for {
		select {
		case <-this.exitCh:
			log.Debugf("%s %p: ask me quit", client.devId, conn)
			break
		default:
		}
		if client.broken {
			log.Debugf("%s %p: client broken", client.devId, conn)
			break
		}

		now := time.Now()
		if now.After(client.lastActive.Add(this.hbTimeout * time.Second)) {
			log.Debugf("%s %p: heartbeat timeout", client.devId, conn)
			break
		}

		conn.SetReadDeadline(now.Add(5 * time.Second))
		if readStep == 0 {
			// read first byte
			n := myRead(conn, headBuf[0:1])
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
			readStep = 1
		}
		if readStep == 1 {
			// read header
			n := myRead(conn, headBuf[nRead:])
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
				log.Warnf("%s %p: header deserialize failed", client.devId, conn)
				break
			}

			if header.Len > this.maxBodyLen {
				log.Warnf("%s %p: header len too big %d", client.devId, conn, header.Len)
				break
			}
			if header.Len > 0 {
				dataBuf = make([]byte, header.Len)
				readStep = 2
				nRead = 0
				startTime = time.Now() //begin to read body
			} else {
				readStep = 0
				nRead = 0
				continue
			}
		}
		if readStep == 2 {
			// read body
			n := myRead(conn, dataBuf[nRead:])
			if n < 0 {
				break
			} else if n == 0 {
				continue
			}
			nRead += n
			if uint32(nRead) < header.Len {
				if now.After(startTime.Add(60 * time.Second)) {
					log.Infof("%s %p: read body timeout", client.devId, conn)
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
			log.Warnf("%s %p: unknown message type %d", client.devId, conn, header.Type)
		}
		readStep = 0
		nRead = 0
	}

	// don't use defer to improve performance
	log.Debugf("%s %p: close connection", client.devId, conn)
	for _, regapp := range client.RegApps {
		AMInstance.DelApp(regapp.RegId)
	}
	this.closeClient(client)
	conn.Close()
	this.clientCount--
}

func (this *Server) waitInit(conn *net.TCPConn) *Client {
	// 要求客户端尽快发送初始化消息
	now := time.Now()
	conn.SetReadDeadline(now.Add(20 * time.Second))
	buf := make([]byte, HEADER_SIZE)
	if n := myRead(conn, buf); n <= 0 {
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

	if header.Len > this.maxBodyLen {
		log.Warnf("%p: header len too big: %d", conn, header.Len)
		conn.Close()
		return nil
	}
	data := make([]byte, header.Len)
	if n := myRead(conn, data); n <= 0 {
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
	if !this.checkDeviceId(devid) {
		log.Warnf("%p: invalid device id (%s)", conn, devid)
		conn.Close()
		return nil
	}
	if !this.checkBlacklist(devid, &now) {
		conn.Close()
		return nil
	}
	log.Debugf("%p: RECV INIT (%s) seq(%d)", conn, data, header.Seq)

	waitcnt := 0
	for {
		x := DevicesMap.Get(devid)
		if x != nil {
			if waitcnt >= 5 {
				conn.Close()
				return nil
			}
			waitcnt += 1
			client := x.(*Client)
			client.broken = true
			time.Sleep(2 * time.Second)
			log.Infof("%s %p: wait old connection close", devid, conn)
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
		log.Warnf("%s %p: failed to check device existence: (%s)", devid, conn, err)
		conn.Close()
		return nil
	}
	if serverName != "" {
		log.Warnf("%s %p: has connected with server (%s)", devid, conn, serverName)
		conn.Close()
		return nil
	}

	client := this.createClient(conn, devid)
	if client == nil {
		conn.Close()
		return nil
	}
	reply := InitReplyMessage{
		Result: 0,
		//HB:     30,
		//Reconn: 60,
	}

	// TODO: below code should check carefully
	if request.Sync == 0 {
		// client give me regapp infos
		for _, info := range request.Apps {
			if info.AppId == "" || info.RegId == "" {
				//log.Warnf("appid or regid is empty")
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
		this.handleOfflineMsgs(client, regapp)
	}
	return client
}

func (this *Server) handleOfflineMsgs(client *Client, regapp *RegApp) {
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
			if matchTopics(regapp.Topics, rawMsg.PushParams.Topic, rawMsg.PushParams.TopicOp) {
				ok = true
				break
			}
		default:
			continue
		}

		if ok {
			if len(regapp.SendIds) > 0 {
				// regapp with sendids
				if rawMsg.SendId != "" {
					found := false
					for _, sendid := range regapp.SendIds {
						if sendid == rawMsg.SendId {
							found = true
							break
						}
					}
					if !found {
						continue
					}
				}
			}
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

func (this *Server) checkBlacklist(devId string, now *time.Time) bool {
	if now.After(this.blackUpdate.Add(60 * time.Second)) {
		if blackDevices, err := storage.Instance.SetMembers("db_black_devices"); err != nil {
			sort.Strings(blackDevices)
			this.blackDevices = blackDevices
			this.blackUpdate = *now
		}
	}

	for _, s := range this.blackDevices {
		if s == devId {
			return false
		}
	}
	return true
}

func (this *Server) checkDeviceId(devId string) bool {
	if devId == "" {
		return false
	}
	return true
}
