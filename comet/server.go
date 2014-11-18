package comet

import (
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
	//"strings"
	"encoding/json"
	"github.com/chenyf/push/auth"
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
	ctrl            chan bool
}

type Server struct {
	name          string // unique name of this server
	exitCh        chan bool
	wg            *sync.WaitGroup
	funcMap       map[uint8]MsgHandler
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
		name:          utils.GetLocalIP(),
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
	header := Header{
		Type: msgType,
		Ver:  0,
		Seq:  seq,
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
	if err := storage.Instance.AddDevice(this.name, devid); err != nil {
		log.Infof("failed to put device %s into redis:", devid, err)
		return nil
	}

	client := &Client{
		devId:           devid,
		RegApps:         make(map[string]*RegApp),
		nextSeq:         100,
		lastActive:      time.Now(),
		outMsgs:         make(chan *Pack, 100),
		WaitingChannels: make(map[uint32]chan *Message, 10),
		ctrl:            make(chan bool),
	}
	DevicesMap.Set(devid, client)

	go func() {
		//log.Infof("%p: enter send routine", conn)
		for {
			select {
			case pack := <-client.outMsgs:
				b, _ := pack.msg.Header.Serialize()
				conn.Write(b)
				conn.Write(pack.msg.Data)
				// add reply channel
				if pack.reply != nil {
					client.WaitingChannels[pack.msg.Header.Seq] = pack.reply
				}
				log.Infof("%s: send msg: (%d) (%s)", client.devId, pack.msg.Header.Type, pack.msg.Data)
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
	if err := storage.Instance.RemoveDevice(this.name, client.devId); err != nil {
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
	this.funcMap[MSG_CMD_REPLY] = handleCmdReply

	// keep the data of this node not expired on redis
	go func() {
		for {
			select {
			case <-this.exitCh:
				log.Infof("existing storage refreshing routine")
				return
			case <-time.After(10 * time.Second):
				storage.Instance.RefreshDevices(this.name, 30)
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
	log.Debugf("accept connection from (%s) (%p)", conn.RemoteAddr(), conn)
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
			log.Debugf("ask me quit\n")
			break
		default:
		}

		now := time.Now()
		if now.After(client.lastActive.Add(this.hbTimeout * time.Second)) {
			log.Debugf("%p: heartbeat timeout", conn)
			break
		}

		//conn.SetReadDeadline(time.Now().Add(this.readTimeout))
		conn.SetReadDeadline(now.Add(10 * time.Second))
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
				log.Warnf("%p: header deserialize failed", conn)
				break
			}

			if header.Len > this.maxBodyLen {
				log.Warnf("%p: header len too big %d", conn, header.Len)
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
					log.Infof("%p: read body timeout", conn)
					break
				}
				continue
			}
			log.Debugf("%p: body (%s)", conn, dataBuf)
		}

		if handler, ok := this.funcMap[header.Type]; ok {
			if ret := handler(conn, client, &header, dataBuf); ret < 0 {
				break
			}
		} else {
			log.Warnf("%p: unknown message type %d", conn, header.Type)
		}
		readflag = 0
		nRead = 0
	}

	// don't use defer to improve performance
	log.Debugf("%s: close connection", client.devId)
	for regid, _ := range client.RegApps {
		AMInstance.RemoveApp(regid)
	}
	this.CloseClient(client)
	conn.Close()
	this.clientCount--
}

func handleOfflineMsgs(client *Client, regapp *RegApp) {
	msgs := storage.Instance.GetOfflineMsgs(regapp.AppId, regapp.RegId, regapp.LastMsgId)
	log.Debugf("%s: get %d offline messages: (%s) (>%d)", regapp.DevId, len(msgs), regapp.AppId, regapp.LastMsgId)
	for _, rawMsg := range msgs {
		ok := false
		switch rawMsg.PushType {
		case 1:
			ok = true
		case 2:
			for _, regid := range rawMsg.PushParams.RegId {
				if regapp.RegId == regid {
					ok = true
					break
				}
			}
		case 3:
			if regapp.UserId == "" {
				continue
			}
			for _, uid := range rawMsg.PushParams.UserId {
				if regapp.UserId == uid {
					ok = true
					break
				}
			}
		case 4:
			for _, devid := range rawMsg.PushParams.DevId {
				if client.devId == devid {
					ok = true
					break
				}
			}
		case 5:

		default:
			continue
		}

		if ok {
			msg := PushMessage{
				MsgId:   rawMsg.MsgId,
				AppId:   rawMsg.AppId,
				Type:    rawMsg.MsgType,
				Content: rawMsg.Content,
			}
			b, _ := json.Marshal(msg)
			client.SendMessage(MSG_PUSH, 0, b, nil)
		}
	}
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

	if header.Type != MSG_INIT {
		log.Warnf("%p: not register message, %d", conn, header.Type)
		conn.Close()
		return nil
	}

	log.Debugf("%p: INIT seq (%d) body(%s)", conn, header.Seq, data)
	var msg InitMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Warnf("%p: decode INIT body failed: (%v)", conn, err)
		conn.Close()
		return nil
	}

	devid := msg.DeviceId
	if devid == "" {
		log.Warnf("%p: invalid device id", conn)
		conn.Close()
		return nil
	}

	if DevicesMap.Check(devid) {
		log.Warnf("%p: device (%s) init in this server already", conn, devid)
		conn.Close()
		return nil
	}

	// check if the device Id has connected to other servers
	exist, err := storage.Instance.IsDeviceExist(devid)
	if err != nil {
		log.Errorf("failed to check device existence:", err)
		conn.Close()
		return nil
	}
	if exist {
		log.Warnf("device %s already exist", devid)
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
	}

	if msg.Sync == 0 {
		for _, info := range msg.Apps {
			if info.AppId == "" || info.RegId == "" {
				log.Warnf("appid or regid is empty")
				continue
			}
			regapp := AMInstance.RegisterApp(client.devId, info.RegId, info.AppId, "")
			if regapp != nil {
				client.RegApps[info.RegId] = regapp
			}
		}
	} else {
		// 客户端要求同步服务端的数据
		// 看存储系统中是否有此设备的数据
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
			regapp := AMInstance.AddApp(client.devId, regid, info)
			client.RegApps[regid] = regapp
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

func sendReply(client *Client, msgType uint8, seq uint32, v interface{}) {
	b, _ := json.Marshal(v)
	client.SendMessage(msgType, seq, b, nil)
}

// app注册后，才可以接收消息
func handleRegister(conn *net.TCPConn, client *Client, header *Header, body []byte) int {
	log.Debugf("%s: REGISTER body(%s)", client.devId, body)
	var msg RegisterMessage
	var reply RegisterReplyMessage

	errReply := func(result int, appId string) {
		reply.Result = result
		reply.AppId = appId
		sendReply(client, MSG_REGISTER_REPLY, header.Seq, &reply)
	}

	if err := json.Unmarshal(body, &msg); err != nil {
		log.Warnf("%s: json decode failed: (%v)", client.devId, err)
		errReply(1, "")
		return 0
	}
	if msg.AppId == "" {
		log.Warnf("%s: appid is empty", client.devId)
		errReply(2, msg.AppId)
		return 0
	}

	var rawapp storage.RawApp
	b, err := storage.Instance.HashGet("db_apps", msg.AppId)
	if err != nil {
		log.Warnf("%s: hashget 'db_apps' failed, (%s). appid (%s)", client.devId, msg.AppId)
		errReply(4, msg.AppId)
		return 0
	}
	if b == nil {
		log.Warnf("%s: unknow appid (%s)", client.devId, msg.AppId)
		errReply(5, msg.AppId)
		return 0
	}

	if err := json.Unmarshal(b, &rawapp); err != nil {
		log.Warnf("%s: invalid data from storage. appid (%s)", client.devId, msg.AppId)
		errReply(6, msg.AppId)
		return 0
	}

	var regUid string = ""
	var ok bool
	if msg.Token != "" {
		ok, regUid = auth.Instance.Auth(msg.Token)
		if !ok {
			log.Warnf("%s: auth failed", client.devId)
			errReply(3, msg.AppId)
			return 0
		}
	}

	regid := RegId(client.devId, msg.AppId, regUid)
	//log.Debugf("%s: uid (%s), regid (%s)", client.devId, regUid, regid)
	if _, ok := client.RegApps[regid]; ok {
		// 已经在内存中，直接返回
		reply.Result = 0
		reply.AppId = msg.AppId
		reply.Pkg = rawapp.Pkg
		reply.RegId = regid
		sendReply(client, MSG_REGISTER_REPLY, header.Seq, &reply)
		return 0
	}

	// 到app管理中心去注册
	regapp := AMInstance.RegisterApp(client.devId, regid, msg.AppId, regUid)
	if regapp == nil {
		log.Warnf("%s: AMInstance register app failed", client.devId)
		errReply(5, msg.AppId)
		return 0
	}
	// 记录到client管理的hash table中
	client.RegApps[regid] = regapp
	reply.Result = 0
	reply.AppId = msg.AppId
	reply.Pkg = rawapp.Pkg
	reply.RegId = regid
	sendReply(client, MSG_REGISTER_REPLY, header.Seq, &reply)

	// 处理离线消息
	handleOfflineMsgs(client, regapp)
	return 0
}

func handleUnregister(conn *net.TCPConn, client *Client, header *Header, body []byte) int {
	log.Debugf("%s: UNREGISTER body(%s)", client.devId, body)
	var msg UnregisterMessage
	var reply UnregisterReplyMessage

	errReply := func(result int, appId string) {
		reply.Result = result
		reply.AppId = appId
		sendReply(client, MSG_UNREGISTER_REPLY, header.Seq, &reply)
	}

	if err := json.Unmarshal(body, &msg); err != nil {
		log.Warnf("%s: json decode failed: (%v)", client.devId, err)
		errReply(1, "")
		return 0
	}

	if msg.AppId == "" || msg.RegId == "" {
		log.Warnf("%s: appid or regid is empty", client.devId)
		errReply(2, msg.AppId)
		return 0
	}

	// unknown regid
	var ok bool
	_, ok = client.RegApps[msg.RegId]
	if !ok {
		log.Warnf("%s: unkonw regid %s", client.devId, msg.RegId)
		errReply(3, msg.AppId)
		return 0
	}

	var regUid string = ""
	if msg.Token != "" {
		ok, regUid = auth.Instance.Auth(msg.Token)
		if !ok {
			log.Warnf("%s: auth failed", client.devId)
			errReply(4, msg.AppId)
			return 0
		}
	}
	//log.Debugf("%s: uid is (%s)", client.devId, uid)
	AMInstance.UnregisterApp(client.devId, msg.RegId, msg.AppId, regUid)
	delete(client.RegApps, msg.RegId)

	reply.Result = 0
	reply.AppId = msg.AppId
	reply.RegId = msg.RegId
	sendReply(client, MSG_UNREGISTER_REPLY, header.Seq, &reply)
	return 0
}

func handlePushReply(conn *net.TCPConn, client *Client, header *Header, body []byte) int {
	log.Debugf("%s: PUSH_REPLY body(%s)", client.devId, body)
	var msg PushReplyMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		log.Warnf("%s: json decode failed: (%v)", client.devId, err)
		return -1
	}

	if msg.AppId == "" || msg.RegId == "" {
		log.Warnf("%s: appid or regis is empty", client.devId)
		return -1
	}
	// unknown regid
	regapp, ok := client.RegApps[msg.RegId]
	if !ok {
		log.Warnf("%s: unkonw regid %s", client.devId, msg.RegId)
		return 0
	}

	if msg.MsgId <= regapp.LastMsgId {
		log.Warnf("%s: msgid mismatch: %d <= %d", client.devId, msg.MsgId, regapp.LastMsgId)
		return 0
	}
	info := &AppInfo{
		AppId:     regapp.AppId,
		UserId:    regapp.UserId,
		Topics:    regapp.Topics,
		LastMsgId: msg.MsgId,
	}
	if ok := AMInstance.UpdateAppInfo(client.devId, msg.RegId, info); !ok {
	}
	AMInstance.UpdateMsgStat(client.devId, msg.MsgId)
	regapp.LastMsgId = msg.MsgId
	return 0
}

func handleCmdReply(conn *net.TCPConn, client *Client, header *Header, body []byte) int {
	log.Debugf("%s: CMD_REPLY body(%s)", client.devId, body)

	ch, ok := client.WaitingChannels[header.Seq]
	if ok {
		//remove waiting channel from map
		delete(client.WaitingChannels, header.Seq)
		ch <- &Message{Header: *header, Data: body}
	} else {
		log.Warnf("no waiting channel for seq: %d, device: %s", header.Seq, client.devId)
	}
	return 0
}

/*
** return:
**   1: invalid JSON
**   2: missing 'appid' or 'regid'
**   3: unknown 'regid'
**   4: too many topics
**   5: storage I/O failed
 */
func handleSubscribe(conn *net.TCPConn, client *Client, header *Header, body []byte) int {
	log.Debugf("%s: SUBSCRIBE body(%s)", client.devId, body)
	var msg SubscribeMessage
	var reply SubscribeReplyMessage

	errReply := func(result int, appId string) {
		reply.Result = result
		reply.AppId = appId
		sendReply(client, MSG_SUBSCRIBE_REPLY, header.Seq, &reply)
	}

	if err := json.Unmarshal(body, &msg); err != nil {
		log.Warnf("%s: json decode failed: (%v)", client.devId, err)
		errReply(1, msg.AppId)
		return 0
	}

	if msg.AppId == "" || msg.RegId == "" {
		log.Warnf("%s: appid or regid is empty", client.devId)
		errReply(2, msg.AppId)
		return 0
	}

	// unknown regid
	var ok bool
	regapp, ok := client.RegApps[msg.RegId]
	if !ok {
		log.Warnf("%s: unkonw regid %s", client.devId, msg.RegId)
		errReply(3, msg.AppId)
		return 0
	}
	for _, item := range regapp.Topics {
		if item == msg.Topic {
			reply.Result = 0
			reply.AppId = msg.AppId
			reply.RegId = msg.RegId
			sendReply(client, MSG_SUBSCRIBE_REPLY, header.Seq, &reply)
			return 0
		}
	}
	if len(regapp.Topics) >= 10 {
		errReply(4, msg.AppId)
		return 0
	}
	topics := append(regapp.Topics, msg.Topic)
	info := &AppInfo{
		AppId:     regapp.AppId,
		UserId:    regapp.UserId,
		Topics:    topics,
		LastMsgId: regapp.LastMsgId,
	}
	if ok := AMInstance.UpdateAppInfo(client.devId, msg.RegId, info); !ok {
		errReply(5, msg.AppId)
		return 0
	}
	regapp.Topics = topics
	reply.Result = 0
	reply.AppId = msg.AppId
	reply.RegId = msg.RegId
	sendReply(client, MSG_SUBSCRIBE_REPLY, header.Seq, &reply)
	return 0
}

/*
** return:
**   1: invalid JSON
**   2: missing 'appid' or 'regid'
**   3: unknown 'regid'
**   4: storage I/O failed
 */
func handleUnsubscribe(conn *net.TCPConn, client *Client, header *Header, body []byte) int {
	log.Debugf("%s: UNSUBSCRIBE body(%s)", client.devId, body)
	var msg UnsubscribeMessage
	var reply UnsubscribeReplyMessage

	errReply := func(result int, appId string) {
		reply.Result = result
		reply.AppId = appId
		sendReply(client, MSG_UNSUBSCRIBE_REPLY, header.Seq, &reply)
	}

	if err := json.Unmarshal(body, &msg); err != nil {
		log.Warnf("%s: json decode failed: (%v)", client.devId, err)
		errReply(1, msg.AppId)
		return 0
	}

	if msg.AppId == "" || msg.RegId == "" {
		log.Warnf("%s: appid or regid is empty", client.devId)
		errReply(2, msg.AppId)
		return 0
	}

	// unknown regid
	var ok bool
	regapp, ok := client.RegApps[msg.RegId]
	if !ok {
		log.Warnf("%s: unkonw regid %s", client.devId, msg.RegId)
		errReply(3, msg.AppId)
		return 0
	}
	index := -1
	for n, item := range regapp.Topics {
		if item == msg.Topic {
			index = n
		}
	}
	if index >= 0 {
		topics := append(regapp.Topics[:index], regapp.Topics[index+1:]...)
		info := &AppInfo{
			AppId:     regapp.AppId,
			UserId:    regapp.UserId,
			Topics:    topics,
			LastMsgId: regapp.LastMsgId,
		}
		if ok := AMInstance.UpdateAppInfo(client.devId, msg.RegId, info); !ok {
			errReply(4, msg.AppId)
			return 0
		}
		regapp.Topics = topics
	}
	reply.Result = 0
	reply.AppId = msg.AppId
	reply.RegId = msg.RegId
	sendReply(client, MSG_UNSUBSCRIBE_REPLY, header.Seq, &reply)
	return 0
}

func handleHeartbeat(conn *net.TCPConn, client *Client, header *Header, body []byte) int {
	//log.Debugf("%s: HEARTBEAT", client.devId)
	client.lastActive = time.Now()
	return 0
}
