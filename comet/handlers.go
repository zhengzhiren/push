package comet

import (
	"encoding/json"
	"net"
	"time"

	"github.com/chenyf/push/auth"
	"github.com/chenyf/push/stats"
	"github.com/chenyf/push/storage"
	log "github.com/cihub/seelog"
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
func handleRegister(conn *net.TCPConn, client *Client, header *Header, body []byte) int {
	log.Debugf("%s: REGISTER body(%s)", client.devId, body)
	var request RegisterMessage
	var reply RegisterReplyMessage

	onReply := func(result int, appId string, pkg string, regId string) {
		reply.Result = result
		reply.AppId = appId
		reply.Pkg = pkg
		reply.RegId = regId
		sendReply(client, MSG_REGISTER_REPLY, header.Seq, &reply)
	}

	if err := json.Unmarshal(body, &request); err != nil {
		log.Warnf("%s: json decode failed: (%v)", client.devId, err)
		onReply(1, "", "", "")
		return 0
	}
	if request.AppId == "" {
		log.Warnf("%s: appid is empty", client.devId)
		onReply(2, request.AppId, "", "")
		return 0
	}

	var rawapp storage.RawApp
	b, err := storage.Instance.HashGet("db_apps", request.AppId)
	if err != nil {
		log.Warnf("%s: hashget 'db_apps' failed, (%s). appid (%s)", client.devId, request.AppId)
		onReply(4, request.AppId, "", "")
		return 0
	}
	if b == nil {
		log.Warnf("%s: unknow appid (%s)", client.devId, request.AppId)
		onReply(5, request.AppId, "", "")
		return 0
	}

	if err := json.Unmarshal(b, &rawapp); err != nil {
		log.Warnf("%s: invalid data from storage. appid (%s)", client.devId, request.AppId)
		onReply(6, request.AppId, "", "")
		return 0
	}

	var reguid string = ""
	var ok bool
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
	}

	regid := RegId(client.devId, request.AppId, reguid)
	//log.Debugf("%s: uid (%s), regid (%s)", client.devId, reguid, regid)
	if regapp, ok := client.RegApps[request.AppId]; ok {
		// 已经在内存中，直接返回
		if regapp.RegId == regid {
			onReply(0, request.AppId, rawapp.Pkg, regid)
			return 0
		} else {
			onReply(10, request.AppId, rawapp.Pkg, regid)
			return 0
		}
	}

	regapp := AMInstance.RegisterApp(client.devId, regid, request.AppId, reguid)
	if regapp == nil {
		log.Warnf("%s: AMInstance register app failed", client.devId)
		onReply(5, request.AppId, "", "")
		return 0
	}
	// 记录到client自己的hash table中
	client.RegApps[request.AppId] = regapp
	onReply(0, request.AppId, rawapp.Pkg, regid)

	// 处理离线消息
	handleOfflineMsgs(client, regapp)
	return 0
}

func handleUnregister(conn *net.TCPConn, client *Client, header *Header, body []byte) int {
	log.Debugf("%s: UNREGISTER body(%s)", client.devId, body)
	var request UnregisterMessage
	var reply UnregisterReplyMessage

	onReply := func(result int, appId string) {
		reply.Result = result
		reply.AppId = appId
		sendReply(client, MSG_UNREGISTER_REPLY, header.Seq, &reply)
	}

	if err := json.Unmarshal(body, &request); err != nil {
		log.Warnf("%s: json decode failed: (%v)", client.devId, err)
		onReply(1, "")
		return 0
	}

	if request.AppId == "" || request.RegId == "" {
		log.Warnf("%s: appid or regid is empty", client.devId)
		onReply(2, request.AppId)
		return 0
	}

	// unknown regid
	var ok bool
	regapp, ok := client.RegApps[request.AppId]
	if !ok {
		log.Warnf("%s: unknown 'appid' %s", client.devId, request.AppId)
		onReply(3, request.AppId)
		return 0
	}
	if regapp.RegId != request.RegId {
		log.Warnf("%s: unmatch 'appid' & 'regid' %s %s", client.devId, request.AppId, request.RegId)
		onReply(10, request.AppId)
		return 0
	}
	AMInstance.UnregisterApp(client.devId, request.RegId, request.AppId, regapp.UserId)
	delete(client.RegApps, request.AppId)
	onReply(0, request.AppId)
	return 0
}

func handlePushReply(conn *net.TCPConn, client *Client, header *Header, body []byte) int {
	log.Debugf("%s: PUSH_REPLY body(%s)", client.devId, body)
	var request PushReplyMessage
	if err := json.Unmarshal(body, &request); err != nil {
		log.Warnf("%s: json decode failed: (%v)", client.devId, err)
		return -1
	}

	if request.AppId == "" || request.RegId == "" {
		log.Warnf("%s: appid or regis is empty", client.devId)
		return -1
	}
	// unknown regid
	regapp, ok := client.RegApps[request.AppId]
	if !ok {
		log.Warnf("%s: unkonw regid %s", client.devId, request.RegId)
		return 0
	}
	if regapp.RegId != request.RegId {
		return 0
	}

	if request.MsgId <= regapp.LastMsgId {
		log.Warnf("%s: msgid mismatch: %d <= %d", client.devId, request.MsgId, regapp.LastMsgId)
		return 0
	}
	info := regapp.AppInfo
	info.LastMsgId = request.MsgId
	AMInstance.UpdateAppInfo(client.devId, request.RegId, &info)
	AMInstance.UpdateMsgStat(client.devId, request.MsgId)
	regapp.LastMsgId = request.MsgId
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
		stats.ReplyTooLate()
	}
	return 0
}

// app注册后，才可以接收消息
func handleRegister2(conn *net.TCPConn, client *Client, header *Header, body []byte) int {
	log.Debugf("%s: REGISTER2 body(%s)", client.devId, body)
	var request Register2Message
	var reply Register2ReplyMessage

	onReply := func(result int, appId string, pkg string, regId string) {
		reply.Result = result
		reply.AppId = appId
		reply.Pkg = pkg
		reply.RegId = regId
		sendReply(client, MSG_REGISTER2_REPLY, header.Seq, &reply)
	}

	if err := json.Unmarshal(body, &request); err != nil {
		log.Warnf("%s: json decode failed: (%v)", client.devId, err)
		onReply(1, "", "", "")
		return 0
	}
	// check 'pkg'
	if request.Pkg == "" {
		log.Warnf("%s: 'pkg' is empty", client.devId)
		onReply(2, "", "", "")
		return 0
	}
	// check 'sendids'
	if len(request.SendIds) == 0 {
		log.Warnf("%s: 'sendids' is empty", client.devId)
		onReply(3, "", request.Pkg, "")
		return 0
	}

	// got appid by pkg
	val, _ := storage.Instance.HashGet("db_packages", request.Pkg)
	if val == nil {
		log.Warnf("%s: no such pkg '%s'", client.devId, request.Pkg)
		onReply(4, "", request.Pkg, "")
		return 0
	}
	appid := string(val)
	regid := RegId(client.devId, appid, request.Uid)

	if regapp, ok := client.RegApps[appid]; ok {
		// 已经在内存中，修改sendids后返回
		if regapp.RegId == regid {
			if ok := addSendids(client, regid, regapp, request.SendIds); !ok {
				onReply(6, appid, request.Pkg, regid)
				return 0
			}
			onReply(0, appid, request.Pkg, regid)
			return 0
		} else {
			log.Warnf("%s: AMInstance register app failed", client.devId)
			onReply(10, appid, request.Pkg, regid)
			return 0
		}
	}

	// 内存中没有，先到app管理中心去注册
	regapp := AMInstance.RegisterApp(client.devId, regid, appid, request.Uid)
	if regapp == nil {
		log.Warnf("%s: AMInstance register app failed", client.devId)
		onReply(7, appid, request.Pkg, regid)
		return 0
	}
	// 记录到client管理的hash table中
	client.RegApps[appid] = regapp

	if ok := addSendids(client, regid, regapp, request.SendIds); !ok {
		onReply(6, appid, request.Pkg, regid)
		return 0
	}
	onReply(0, appid, request.Pkg, regid)

	// 处理离线消息
	handleOfflineMsgs(client, regapp)
	return 0
}

func handleUnregister2(conn *net.TCPConn, client *Client, header *Header, body []byte) int {
	log.Debugf("%s: UNREGISTER2 body(%s)", client.devId, body)
	var request Unregister2Message
	var reply Unregister2ReplyMessage

	onReply := func(result int, appId string, pkg string, senderCnt int) {
		reply.Result = result
		reply.AppId = appId
		reply.Pkg = pkg
		reply.SenderCnt = senderCnt
		sendReply(client, MSG_UNREGISTER2_REPLY, header.Seq, &reply)
	}

	if err := json.Unmarshal(body, &request); err != nil {
		log.Warnf("%s: json decode failed: (%v)", client.devId, err)
		onReply(1, "", "", 0)
		return 0
	}

	if request.Pkg == "" {
		log.Warnf("%s: 'pkg' is empty", client.devId)
		onReply(2, "", "", 0)
		return 0
	}
	if request.SendId == "" {
		log.Warnf("%s: 'sendid' is empty", client.devId)
		onReply(3, "", request.Pkg, 0)
		return 0
	}

	// FUCK: no 'appid', no 'regid'; got them by request.Pkg
	val, _ := storage.Instance.HashGet("db_packages", request.Pkg)
	if val == nil {
		log.Warnf("%s: no such pkg '%s'", client.devId, request.Pkg)
		onReply(4, "", request.Pkg, 0)
		return 0
	}
	appid := string(val)
	regid := RegId(client.devId, appid, request.Uid)
	regapp, ok := client.RegApps[appid]
	if !ok {
		log.Warnf("%s: 'pkg' %s hasn't register %s", client.devId, request.Pkg)
		onReply(5, appid, request.Pkg, 0)
		return 0
	}
	if regapp.RegId != regid {
		log.Warnf("%s: 'pkg' %s hasn't register %s", client.devId, request.Pkg)
		onReply(10, appid, request.Pkg, 0)
		return 0
	}
	if ok := delSendid(client, regid, regapp, request.SendId); !ok {
		onReply(6, appid, request.Pkg, 0)
		return 0
	}
	if len(regapp.SendIds) != 0 {
		onReply(0, appid, request.Pkg, len(regapp.SendIds))
		return 0
	}

	// remove it from memory when regapp.SendIds is empty
	AMInstance.UnregisterApp(client.devId, regid, appid, request.Uid)
	delete(client.RegApps, appid)
	onReply(0, appid, request.Pkg, 0)
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
	log.Debugf("%s: SUBSCRIBE body(%s)", client.devId, body)
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

	// unknown regid
	var ok bool
	regapp, ok := client.RegApps[request.AppId]
	if !ok {
		log.Warnf("%s: unkonw regid %s", client.devId, request.RegId)
		onReply(4, request.AppId)
		return 0
	}
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
	log.Debugf("%s: UNSUBSCRIBE body(%s)", client.devId, body)
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

	// unknown regid
	var ok bool
	regapp, ok := client.RegApps[request.AppId]
	if !ok {
		log.Warnf("%s: unkonw regid %s", client.devId, request.RegId)
		onReply(3, request.AppId)
		return 0
	}
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
	log.Debugf("%s: GETTOPICS body(%s)", client.devId, body)
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

	// unknown regid
	var ok bool
	regapp, ok := client.RegApps[request.AppId]
	if !ok {
		log.Warnf("%s: unkonw regid %s", client.devId, request.RegId)
		onReply(3, request.AppId)
		return 0
	}
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

func handleHeartbeat(conn *net.TCPConn, client *Client, header *Header, body []byte) int {
	//log.Debugf("%s: HEARTBEAT", client.devId)
	client.lastActive = time.Now()
	return 0
}