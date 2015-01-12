package comet

import (
	"encoding/json"
	"fmt"
	log "github.com/cihub/seelog"
	"net"
	"time"

	"github.com/chenyf/push/auth"
	"github.com/chenyf/push/stats"
	"github.com/chenyf/push/storage"
)

func sendReply(client *Client, msgType uint8, seq uint32, v interface{}) {
	b, _ := json.Marshal(v)
	client.SendMessage(msgType, seq, b, nil)
}

func addSendids(client *Client, regId string, regapp *RegApp, sendids []string) bool {
	var added []string
	for _, s1 := range sendids {
		if s1 == "" {
			continue
		}
		found := false
		for _, s2 := range regapp.SendIds {
			if s1 == s2 {
				found = true
				break
			}
		}
		if !found {
			added = append(added, s1)
		}
	}

	// update 'sendids'
	if len(added) != 0 {
		new_sendids := append(regapp.SendIds, added...)
		info := regapp.AppInfo
		info.SendIds = new_sendids
		if ok := AMInstance.UpdateAppInfo(client.devId, regId, &info); !ok {
			return false
		}
		regapp.SendIds = new_sendids
	}
	return true
}

func delSendid(client *Client, regId string, regapp *RegApp, sendid string) bool {
	new_sendids := []string{}
	if sendid != "[all]" {
		index := -1
		for n, item := range regapp.SendIds {
			if item == sendid {
				index = n
				break
			}
		}
		if index < 0 {
			// not found
			return true
		}
		// delete it
		new_sendids = append(regapp.SendIds[:index], regapp.SendIds[index+1:]...)
	}
	info := regapp.AppInfo
	info.SendIds = new_sendids
	if ok := AMInstance.UpdateAppInfo(client.devId, regId, &info); !ok {
		return false
	}
	regapp.SendIds = new_sendids
	return true
}

// app注册后，才可以接收消息
// 允许重复注册
func handleRegister(conn *net.TCPConn, client *Client, header *Header, body []byte) int {
	log.Debugf("%s %p: RECV Register (%s)", client.devId, conn, body)
	var request RegisterMessage
	var reply RegisterReplyMessage

	onReply := func(result int, msg string, appId string, pkg string, regId string) {
		reply.Result = result
		if result != 0 {
			reply.ErrInfo = msg
		}
		reply.AppId = appId
		reply.Pkg = pkg
		reply.RegId = regId
		sendReply(client, MSG_REGISTER_REPLY, header.Seq, &reply)
	}

	if err := json.Unmarshal(body, &request); err != nil {
		log.Warnf("%s %p: json decode failed: (%v)", client.devId, conn, err)
		onReply(ERR_INVALID_REQ, "invalid body", "", "", "")
		return 0
	}
	if request.AppId == "" {
		onReply(ERR_INVALID_PARAMS, "'appid' is empty", request.AppId, "", "")
		return 0
	}

	var rawapp storage.RawApp
	b, err := storage.Instance.HashGet("db_apps", request.AppId)
	if err != nil {
		log.Warnf("%s %p: hashget 'db_apps' failed, (%s) (%s)", client.devId, conn, request.AppId, err)
		onReply(ERR_INTERNAL, "server error", request.AppId, "", "")
		return 0
	}
	if b == nil {
		onReply(ERR_INVALID_APPID, "unknown appid", request.AppId, "", "")
		return 0
	}

	if err := json.Unmarshal(b, &rawapp); err != nil {
		log.Warnf("%s %p: invalid data from storage. appid (%s)", client.devId, conn, request.AppId)
		onReply(ERR_INTERNAL, "server error", request.AppId, "", "")
		return 0
	}

	var reguid string = ""
	var ok bool
	/*
		if request.Uid != "" {
			reguid = request.Uid
		} else {
			if request.Token != "" {
				ok, reguid = auth.Instance.Auth(request.Token)
				if !ok {
					log.Warnf("%s: auth failed", client.devId)
					onReply(3, request.AppId, "", "")
					return 0
				}
			}
		}*/
	if request.Token != "" {
		ok, reguid = auth.Instance.Auth(request.Token)
		if !ok {
			log.Warnf("%s %p: auth failed", client.devId, conn)
			onReply(ERR_AUTH, "auth failed", request.AppId, "", "")
			return 0
		}
	}
	regid := RegId(client.devId, request.AppId, reguid)
	//log.Debugf("%s: uid (%s), regid (%s)", client.devId, reguid, regid)
	if regapp, ok := client.RegApps[request.AppId]; ok {
		// 已经在内存中
		if regapp.RegId == regid {
			// regid一致，直接返回注册成功
			onReply(0, "", request.AppId, rawapp.Pkg, regid)
			return 0
		} else {
			// regid不一致，不允许注册
			onReply(ERR_REG_CONFLICT, "reg conflict", request.AppId, rawapp.Pkg, regid)
			return 0
		}
	}

	// 首次注册
	regapp := AMInstance.RegisterApp(client.devId, regid, request.AppId, reguid)
	if regapp == nil {
		log.Warnf("%s %p: AMInstance register app failed", client.devId, conn)
		onReply(ERR_INTERNAL, "server error", request.AppId, "", "")
		return 0
	}
	// 记录到client自己的hash table中
	client.RegApps[request.AppId] = regapp
	onReply(0, "", request.AppId, rawapp.Pkg, regid)

	// 处理离线消息
	//handleOfflineMsgs(client, regapp)
	return 0
}

func handleUnregister(conn *net.TCPConn, client *Client, header *Header, body []byte) int {
	log.Debugf("%s %p: RECV Unregister (%s)", client.devId, conn, body)
	var request UnregisterMessage
	var reply UnregisterReplyMessage

	onReply := func(result int, msg string, appId string) {
		reply.Result = result
		if result != 0 {
			reply.ErrInfo = msg
		}
		reply.AppId = appId
		sendReply(client, MSG_UNREGISTER_REPLY, header.Seq, &reply)
	}

	if err := json.Unmarshal(body, &request); err != nil {
		log.Warnf("%s %p: json decode failed: (%v)", client.devId, conn, err)
		onReply(ERR_INVALID_REQ, "invalid body", "")
		return 0
	}

	if request.AppId == "" {
		onReply(ERR_INVALID_PARAMS, "'appid' is empty", request.AppId)
		return 0
	}
	if request.RegId == "" {
		onReply(ERR_INVALID_PARAMS, "'regid' is empty", request.AppId)
		return 0
	}

	var ok bool
	regapp, ok := client.RegApps[request.AppId]
	if !ok {
		onReply(ERR_NOT_REG, "hasn't register", request.AppId)
		return 0
	}
	if regapp.RegId != request.RegId {
		onReply(ERR_INVALID_REGID, fmt.Sprintf("registered with regid %s", regapp.RegId), request.AppId)
		return 0
	}
	AMInstance.UnregisterApp(client.devId, request.RegId, request.AppId, regapp.UserId)
	delete(client.RegApps, request.AppId)
	onReply(0, "", request.AppId)
	return 0
}

type UpdateJob struct {
	devid string
	regid string
	info  AppInfo
}

func (this *UpdateJob) Do() bool {
	AMInstance.UpdateAppInfo(this.devid, this.regid, &this.info)
	storage.Instance.MsgStatsReceived(this.info.LastMsgId)
	storage.Instance.AppStatsReceived(this.info.AppId)
	return true

}

func handlePushReply(conn *net.TCPConn, client *Client, header *Header, body []byte) int {
	log.Debugf("%s %p: RECV PushReply (%s)", client.devId, conn, body)
	var request PushReplyMessage
	if err := json.Unmarshal(body, &request); err != nil {
		log.Warnf("%s %p: json decode failed: (%v)", client.devId, conn, err)
		return 0
	}

	if request.AppId == "" || request.RegId == "" {
		log.Warnf("%s %p: appid or regid is empty", client.devId, conn)
		return 0
	}
	// unknown regid
	regapp, ok := client.RegApps[request.AppId]
	if !ok {
		log.Warnf("%s %p: unknown regid %s", client.devId, conn, request.RegId)
		return 0
	}
	if regapp.RegId != request.RegId {
		return 0
	}

	if request.MsgId <= regapp.LastMsgId {
		log.Warnf("%s %p: msgid mismatch: %d <= %d", client.devId, conn, request.MsgId, regapp.LastMsgId)
		return 0
	}
	regapp.LastMsgId = request.MsgId
	job := &UpdateJob{
		devid: client.devId,
		regid: request.RegId,
		info:  regapp.AppInfo,
	}
	*(client.jobChannel) <- job
	return 0
	/*
		info := regapp.AppInfo
		info.LastMsgId = request.MsgId
		AMInstance.UpdateAppInfo(client.devId, request.RegId, &info)
		storage.Instance.MsgStatsReceived(request.MsgId)
		regapp.LastMsgId = request.MsgId
		return 0
	*/
}

func handleCmdReply(conn *net.TCPConn, client *Client, header *Header, body []byte) int {
	log.Debugf("%s %p: RECV CmdReply (%s)", client.devId, conn, body)

	ch, ok := client.replyChannels[header.Seq]
	if ok {
		//remove waiting channel from map
		delete(client.replyChannels, header.Seq)
		ch <- &Message{Header: header, Data: body}
	} else {
		log.Warnf("%s %p: no waiting channel for seq: %d, device: %s", client.devId, conn, header.Seq)
		stats.ReplyTooLate()
	}
	return 0
}

// app注册后，才可以接收消息
func handleRegister2(conn *net.TCPConn, client *Client, header *Header, body []byte) int {
	log.Debugf("%s %p: RECV Register2 (%s)", client.devId, conn, body)
	var request Register2Message
	var reply Register2ReplyMessage

	onReply := func(result int, msg string, appId string, pkg string, regId string) {
		reply.Result = result
		if result != 0 {
			reply.ErrInfo = msg
		}
		reply.AppId = appId
		reply.Pkg = pkg
		reply.RegId = regId
		sendReply(client, MSG_REGISTER2_REPLY, header.Seq, &reply)
	}

	if err := json.Unmarshal(body, &request); err != nil {
		log.Warnf("%s %p: json decode failed: (%v)", client.devId, conn, err)
		onReply(ERR_INVALID_REQ, "invalid body", "", "", "")
		return 0
	}
	// check 'pkg'
	if request.Pkg == "" {
		log.Warnf("%s %p: 'pkg' is empty", client.devId, conn)
		onReply(ERR_INVALID_PARAMS, "'pkg' is empty", "", "", "")
		return 0
	}
	// check 'sendids'
	if len(request.SendIds) == 0 {
		log.Warnf("%s %p: 'sendids' is empty", client.devId, conn)
		onReply(ERR_INVALID_PARAMS, "'sendids' is empty", "", request.Pkg, "")
		return 0
	}

	// got appid by pkg
	val, _ := storage.Instance.HashGet("db_packages", request.Pkg)
	if val == nil {
		log.Warnf("%s %p: no such pkg '%s'", client.devId, conn, request.Pkg)
		onReply(ERR_INVALID_PKG, "unknown pkg", "", request.Pkg, "")
		return 0
	}
	appid := string(val)

	var reguid string = ""
	var ok bool
	token := request.Token
	/*if token == "" {
		token = request.Uid
	}*/
	if token != "" {
		ok, reguid = auth.Instance.Auth(token)
		if !ok {
			log.Warnf("%s %p: auth failed", client.devId, conn)
			onReply(ERR_AUTH, "auth failed", appid, request.Pkg, "")
			return 0
		}
	}
	regid := RegId(client.devId, appid, reguid)
	if regapp, ok := client.RegApps[appid]; ok {
		// 已经在内存中，修改sendids后返回
		if regapp.RegId == regid {
			if ok := addSendids(client, regid, regapp, request.SendIds); !ok {
				onReply(ERR_INTERNAL, "server error", appid, request.Pkg, regid)
				return 0
			}
			onReply(0, "", appid, request.Pkg, regid)
			return 0
		} else {
			onReply(ERR_REG_CONFLICT, "registered with another regid", appid, request.Pkg, regid)
			return 0
		}
	}

	// 首次注册
	regapp := AMInstance.RegisterApp(client.devId, regid, appid, reguid)
	if regapp == nil {
		log.Warnf("%s %p: AMInstance register app failed", client.devId, conn)
		onReply(ERR_INTERNAL, "server error", appid, request.Pkg, regid)
		return 0
	}
	// 记录到client管理的hash table中
	client.RegApps[appid] = regapp

	if ok := addSendids(client, regid, regapp, request.SendIds); !ok {
		onReply(ERR_INTERNAL, "server error", appid, request.Pkg, regid)
		return 0
	}
	onReply(0, "", appid, request.Pkg, regid)

	// 处理离线消息
	//handleOfflineMsgs(client, regapp)
	return 0
}

func handleUnregister2(conn *net.TCPConn, client *Client, header *Header, body []byte) int {
	log.Debugf("%s %p: RECV Unregister2 (%s)", client.devId, conn, body)
	var request Unregister2Message
	var reply Unregister2ReplyMessage

	onReply := func(result int, msg string, appId string, pkg string, senderCnt int) {
		reply.Result = result
		if result != 0 {
			reply.ErrInfo = msg
		}
		reply.AppId = appId
		reply.Pkg = pkg
		reply.SenderCnt = senderCnt
		sendReply(client, MSG_UNREGISTER2_REPLY, header.Seq, &reply)
	}

	if err := json.Unmarshal(body, &request); err != nil {
		log.Warnf("%s %p: json decode failed: (%v)", client.devId, conn, err)
		onReply(ERR_INVALID_REQ, "invalid body", "", "", 0)
		return 0
	}

	if request.Pkg == "" {
		log.Warnf("%s %p: 'pkg' is empty", client.devId, conn)
		onReply(ERR_INVALID_PARAMS, "'pkg' is empty", "", "", 0)
		return 0
	}
	if request.RegId == "" {
		log.Warnf("%s %p: 'regid' is empty", client.devId, conn)
		onReply(ERR_INVALID_PARAMS, "'regid' is empty", "", "", 0)
		return 0
	}
	if request.SendId == "" {
		log.Warnf("%s %p: 'sendid' is empty", client.devId, conn)
		onReply(ERR_INVALID_PARAMS, "'sendid' is empty", "", request.Pkg, 0)
		return 0
	}

	val, _ := storage.Instance.HashGet("db_packages", request.Pkg)
	if val == nil {
		log.Warnf("%s %p: no such pkg '%s'", client.devId, conn, request.Pkg)
		onReply(ERR_INVALID_PKG, "unknown pkg", "", request.Pkg, 0)
		return 0
	}
	appid := string(val)
	regapp, ok := client.RegApps[appid]
	if !ok {
		log.Warnf("%s %p: 'pkg' %s hasn't register %s", client.devId, conn, request.Pkg)
		onReply(ERR_NOT_REG, "hasn't register", appid, request.Pkg, 0)
		return 0
	}
	if regapp.RegId != request.RegId {
		onReply(ERR_INVALID_REGID, fmt.Sprintf("registered with regid %s", regapp.RegId), appid, request.Pkg, 0)
		return 0
	}
	if ok := delSendid(client, request.RegId, regapp, request.SendId); !ok {
		onReply(ERR_INTERNAL, "server error", appid, request.Pkg, 0)
		return 0
	}
	if len(regapp.SendIds) != 0 {
		onReply(0, "", appid, request.Pkg, len(regapp.SendIds))
		return 0
	}

	// remove it from memory when regapp.SendIds is empty
	AMInstance.UnregisterApp(client.devId, request.RegId, appid, regapp.UserId)
	delete(client.RegApps, appid)
	onReply(0, "", appid, request.Pkg, 0)
	return 0
}

/*
** return:
**   1: invalid JSON
**   2: missing 'appid' or 'regid'
**   3: invalid 'topic' length
**   4: unknown 'regid'
**   5: too many topics
**   6: storage I/O failed
 */
func handleSubscribe(conn *net.TCPConn, client *Client, header *Header, body []byte) int {
	log.Debugf("%s: RECV Subscribe (%s)", client.devId, body)
	var request SubscribeMessage
	var reply SubscribeReplyMessage

	onReply := func(result int, appId string) {
		reply.Result = result
		reply.AppId = appId
		sendReply(client, MSG_SUBSCRIBE_REPLY, header.Seq, &reply)
	}

	if err := json.Unmarshal(body, &request); err != nil {
		log.Warnf("%s: json decode failed: (%v)", client.devId, err)
		onReply(1, request.AppId)
		return 0
	}

	if request.AppId == "" || request.RegId == "" {
		log.Warnf("%s: appid or regid is empty", client.devId)
		onReply(2, request.AppId)
		return 0
	}
	if len(request.Topic) == 0 || len(request.Topic) > 64 {
		log.Warnf("%s: invalid 'topic' length", client.devId)
		onReply(3, request.AppId)
		return 0
	}

	// unknown AppId
	var ok bool
	regapp, ok := client.RegApps[request.AppId]
	if !ok {
		log.Warnf("%s: unkonw AppId %s", client.devId, request.AppId)
		onReply(4, request.AppId)
		return 0
	}
	// unknown RegId
	if regapp.RegId != request.RegId {
		log.Warnf("%s: unkonw regid %s", client.devId, request.RegId)
		onReply(10, request.AppId)
		return 0
	}
	for _, item := range regapp.Topics {
		if item == request.Topic {
			reply.Result = 0
			reply.AppId = request.AppId
			reply.RegId = request.RegId
			sendReply(client, MSG_SUBSCRIBE_REPLY, header.Seq, &reply)
			return 0
		}
	}
	if len(regapp.Topics) >= 10 {
		onReply(5, request.AppId)
		return 0
	}
	topics := append(regapp.Topics, request.Topic)
	info := regapp.AppInfo
	info.Topics = topics
	if ok := AMInstance.UpdateAppInfo(client.devId, request.RegId, &info); !ok {
		onReply(6, request.AppId)
		return 0
	}
	regapp.Topics = topics
	reply.Result = 0
	reply.AppId = request.AppId
	reply.RegId = request.RegId
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
	log.Debugf("%s: RECV Unsubscribe (%s)", client.devId, body)
	var request UnsubscribeMessage
	var reply UnsubscribeReplyMessage

	onReply := func(result int, appId string) {
		reply.Result = result
		reply.AppId = appId
		sendReply(client, MSG_UNSUBSCRIBE_REPLY, header.Seq, &reply)
	}

	if err := json.Unmarshal(body, &request); err != nil {
		log.Warnf("%s: json decode failed: (%v)", client.devId, err)
		onReply(1, request.AppId)
		return 0
	}

	if request.AppId == "" || request.RegId == "" {
		log.Warnf("%s: appid or regid is empty", client.devId)
		onReply(2, request.AppId)
		return 0
	}

	// unknown AppId
	var ok bool
	regapp, ok := client.RegApps[request.AppId]
	if !ok {
		log.Warnf("%s: unkonw AppId %s", client.devId, request.AppId)
		onReply(3, request.AppId)
		return 0
	}
	// unknown regid
	if regapp.RegId != request.RegId {
		log.Warnf("%s: unkonw regid %s", client.devId, request.RegId)
		onReply(10, request.AppId)
		return 0
	}
	index := -1
	for n, item := range regapp.Topics {
		if item == request.Topic {
			index = n
		}
	}
	if index >= 0 {
		topics := append(regapp.Topics[:index], regapp.Topics[index+1:]...)
		info := regapp.AppInfo
		info.Topics = topics
		if ok := AMInstance.UpdateAppInfo(client.devId, request.RegId, &info); !ok {
			onReply(4, request.AppId)
			return 0
		}
		regapp.Topics = topics
	}
	reply.Result = 0
	reply.AppId = request.AppId
	reply.RegId = request.RegId
	sendReply(client, MSG_UNSUBSCRIBE_REPLY, header.Seq, &reply)
	return 0
}

func handleGetTopics(conn *net.TCPConn, client *Client, header *Header, body []byte) int {
	log.Debugf("%s: RECV GetTopics (%s)", client.devId, body)
	var request GetTopicsMessage
	var reply GetTopicsReplyMessage

	onReply := func(result int, appId string) {
		reply.Result = result
		reply.AppId = appId
		sendReply(client, MSG_GET_TOPICS_REPLY, header.Seq, &reply)
	}

	if err := json.Unmarshal(body, &request); err != nil {
		log.Warnf("%s: json decode failed: (%v)", client.devId, err)
		onReply(1, request.AppId)
		return 0
	}

	if request.AppId == "" || request.RegId == "" {
		log.Warnf("%s: appid or regid is empty", client.devId)
		onReply(2, request.AppId)
		return 0
	}

	// unknown AppId
	var ok bool
	regapp, ok := client.RegApps[request.AppId]
	if !ok {
		log.Warnf("%s: unkonw AppId %s", client.devId, request.AppId)
		onReply(3, request.AppId)
		return 0
	}

	// unknown RegId
	if regapp.RegId != request.RegId {
		log.Warnf("%s: unkonw regid %s", client.devId, request.RegId)
		onReply(10, request.AppId)
		return 0
	}
	reply.Result = 0
	reply.AppId = request.AppId
	reply.RegId = request.RegId
	reply.Topics = regapp.Topics
	sendReply(client, MSG_GET_TOPICS_REPLY, header.Seq, &reply)
	return 0
}

/*
** return:
**   1: invalid JSON
**   2: missing 'appid' or 'regid'
**   3: unknown 'regid'
 */
func handleStats(conn *net.TCPConn, client *Client, header *Header, body []byte) int {
	log.Debugf("%s: RECV Stats (%s)", client.devId, body)
	var request StatsMessage
	var reply StatsReplyMessage

	onReply := func(result int, appId string) {
		reply.Result = result
		reply.AppId = appId
		sendReply(client, MSG_STATS_REPLY, header.Seq, &reply)
	}

	if err := json.Unmarshal(body, &request); err != nil {
		log.Warnf("%s: json decode failed: (%v)", client.devId, err)
		onReply(1, request.AppId)
		return 0
	}

	if request.AppId == "" || request.RegId == "" {
		log.Warnf("%s: appid or regid is empty", client.devId)
		onReply(2, request.AppId)
		return 0
	}

	// unknown AppId
	var ok bool
	regapp, ok := client.RegApps[request.AppId]
	if !ok {
		log.Warnf("%s: unkonw AppId %s", client.devId, request.AppId)
		onReply(4, request.AppId)
		return 0
	}
	// unknown RegId
	if regapp.RegId != request.RegId {
		log.Warnf("%s: unkonw regid %s", client.devId, request.RegId)
		onReply(10, request.AppId)
		return 0
	}

	if request.Click {
		storage.Instance.MsgStatsClick(request.MsgId)
		storage.Instance.AppStatsClick(request.AppId)
	}
	reply.Result = 0
	reply.AppId = request.AppId
	reply.RegId = request.RegId
	sendReply(client, MSG_STATS_REPLY, header.Seq, &reply)
	return 0
}

func handleHeartbeat(conn *net.TCPConn, client *Client, header *Header, body []byte) int {
	//log.Debugf("%s: HEARTBEAT", client.devId)
	client.lastActive = time.Now()
	return 0
}
