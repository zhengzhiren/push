package comet

import (
	//"fmt"
	"encoding/json"
	"github.com/chenyf/push/storage"
	log "github.com/cihub/seelog"
)

const (
	PUSH_TYPE_ALL    = 1
	PUSH_TYPE_REGID  = 2
	PUSH_TYPE_USERID = 3
	PUSH_TYPE_DEVID  = 4
	PUSH_TYPE_TOPIC  = 5
	PUSH_TYPE_ALIAS  = 6

	MSG_TYPE_NOTIFICATION = 1
	MSG_TYPE_MESSAGE      = 2
)

//func SendCommand(devId string, cmd *CommandMessage, correlationId, callbackQueue string) bool {
//	wait := 5
//	client := DevicesMap.Get(devId).(*Client)
//	if client == nil {
//		return false
//	}
//	var replyChannel chan *Message = nil
//	if wait > 0 {
//		replyChannel = make(chan *Message)
//		if wait > 10 {
//			wait = 10
//		}
//	}
//	bCmd, _ := json.Marshal(cmd)
//	seq, ok := client.SendMessage(MSG_CMD, 0, bCmd, replyChannel)
//	if !ok {
//		return false
//	}
//	if wait <= 0 {
//		return true
//	}
//	select {
//	case reply := <-replyChannel:
//		var resp CommandReplyMessage
//		err := json.Unmarshal(reply.Data, &resp)
//		if err != nil {
//			log.Errorf("Bad command reply message: %s", err)
//			return false
//		}
//		mq_rpc.RpcServer.SendRpcResponse(callbackQueue, correlationId, resp.Result)
//		return true
//	case <-time.After(time.Duration(wait) * time.Second):
//		client.MsgTimeout(seq)
//		return false
//	}
//	return false
//}

func pushMessage(appId string, app *RegApp, rawMsg *storage.RawMessage, header *Header, body []byte) bool {
	//if len(app.SendIds) != 0 {
	// regapp with sendids
	log.Infof("msgid %d: before push to (device %s) (regid %s)", rawMsg.MsgId, app.DevId, app.RegId)
	if rawMsg.SendId != "" {
		found := false
		for _, sendid := range app.SendIds {
			if sendid == rawMsg.SendId {
				found = true
				break
			}
		}
		if !found {
			log.Debugf("msgid %d: check sendid (%s) failed", rawMsg.MsgId, rawMsg.SendId)
			return false
		}
	}

	x := DevicesMap.Get(app.DevId)
	if x == nil {
		log.Debugf("msgid %d: device %s offline", rawMsg.MsgId, app.DevId)
		return false
	}
	client := x.(*Client)
	client.SendMessage2(header, body)
	log.Infof("msgid %d: after push to (device %s) (regid %s)", rawMsg.MsgId, app.DevId, app.RegId)
	storage.Instance.MsgStatsSend(rawMsg.MsgId)
	storage.Instance.AppStatsSend(rawMsg.AppId)
	return true
}

func PushMessages(appId string, rawMsg *storage.RawMessage) error {
	msg := PushMessage{
		MsgId: rawMsg.MsgId,
		AppId: appId,
		Type:  rawMsg.MsgType,
	}
	if rawMsg.MsgType == MSG_TYPE_MESSAGE {
		msg.Content = rawMsg.Content
	} else {
		b, _ := json.Marshal(rawMsg.Notification)
		msg.Content = string(b)
	}

	body, err := json.Marshal(msg)
	if err != nil {
		log.Errorf("msgid %d: failed to encode msg %v", rawMsg.MsgId, msg)
		return err
	}
	header := makeHeader(MSG_PUSH, 0, uint32(len(body)))

	switch rawMsg.PushType {
	case PUSH_TYPE_ALL: // broadcast
		apps := AMInstance.GetApps(appId)
		for _, app := range apps {
			pushMessage(appId, app, rawMsg, header, body)
		}
		log.Infof("msgid %d: get %d apps", rawMsg.MsgId, len(apps))
	case PUSH_TYPE_REGID: // regid list
		count := 0
		for _, regid := range rawMsg.PushParams.RegId {
			app := AMInstance.GetApp(appId, regid)
			if app != nil {
				pushMessage(appId, app, rawMsg, header, body)
				count += 1
			}
		}
		log.Infof("msgid %d: get %d apps by regid", rawMsg.MsgId, count)
	case PUSH_TYPE_USERID: // userid list
		for _, uid := range rawMsg.PushParams.UserId {
			count := 0
			apps := AMInstance.GetAppsByUser(appId, uid)
			for _, app := range apps {
				pushMessage(appId, app, rawMsg, header, body)
				count += 1
			}
			log.Infof("msgid %d: get %d apps by user %s", rawMsg.MsgId, count, uid)
		}
	case PUSH_TYPE_DEVID: // devid list
		count := 0
		for _, devid := range rawMsg.PushParams.DevId {
			app := AMInstance.GetAppByDevice(appId, devid)
			if app != nil {
				pushMessage(appId, app, rawMsg, header, body)
				count += 1
			}
		}
		log.Infof("msgid %d: get %d apps by devid", rawMsg.MsgId, count)
	case PUSH_TYPE_TOPIC: // topic
		apps := AMInstance.GetAppsByTopic(appId, rawMsg.PushParams.Topic, rawMsg.PushParams.TopicOp)
		for _, app := range apps {
			pushMessage(appId, app, rawMsg, header, body)
		}
	default:
	}
	return nil
}
