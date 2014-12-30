package comet

import (
	//"fmt"
	"encoding/json"
	"github.com/chenyf/push/storage"
	log "github.com/cihub/seelog"
)

const (
	PUSH_TYPE_ALL       = 1
	PUSH_TYPE_REGID     = 2
	PUSH_TYPE_USERID    = 3
	PUSH_TYPE_DEVID     = 4
	PUSH_TYPE_TOPIC     = 5
	PUSH_TYPE_ALIAS     = 6

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

func pushMessage(appId string, app *RegApp, rawMsg *storage.RawMessage, msg *PushMessage) bool {
	//if len(app.SendIds) != 0 {
	// regapp with sendids
	log.Infof("before push to (app %s) (device %s) (regid %s)", appId, app.DevId, app.RegId)
	if rawMsg.SendId != "" {
		found := false
		for _, sendid := range(app.SendIds) {
			if sendid == rawMsg.SendId {
				found = true
				break
			}
		}
		if !found {
			log.Debugf("check sendid (%s) failed", rawMsg.SendId)
			return false
		}
	}

	x := DevicesMap.Get(app.DevId)
	if x == nil {
		log.Debugf("device %s offline", app.DevId)
		return false
	}
	client := x.(*Client)
	b, err := json.Marshal(msg)
	if err != nil {
		log.Infof("failed to encode msg %v", msg)
		return false
	}
	client.SendMessage(MSG_PUSH, 0, b, nil)
	log.Infof("after push to (app %s) (device %s) (regid %s)", appId, app.DevId, app.RegId)
	return true
}

func PushMessages(appId string, rawMsg *storage.RawMessage) error {
	msg := PushMessage{
		MsgId:   rawMsg.MsgId,
		AppId:   appId,
		Type:    rawMsg.MsgType,
	}
	if rawMsg.MsgType == MSG_TYPE_MESSAGE {
		msg.Content = rawMsg.Content
	} else {
		b, _ := json.Marshal(rawMsg.Notification)
		msg.Content = string(b)
	}
	switch rawMsg.PushType {
	case PUSH_TYPE_ALL: // broadcast
		apps := AMInstance.GetApps(appId)
		for _, app := range apps {
			pushMessage(appId, app, rawMsg, &msg)
		}
	case PUSH_TYPE_REGID: // regid list
		for _, regid := range rawMsg.PushParams.RegId {
			app := AMInstance.GetApp(appId, regid)
			if app != nil {
				pushMessage(appId, app, rawMsg, &msg)
			}
		}
	case PUSH_TYPE_USERID: // userid list
		for _, uid := range rawMsg.PushParams.UserId {
			apps := AMInstance.GetAppsByUser(appId, uid)
			for _, app := range apps {
				pushMessage(appId, app, rawMsg, &msg)
			}
		}
	case PUSH_TYPE_DEVID: // devid list
		for _, devid := range rawMsg.PushParams.DevId {
			app := AMInstance.GetAppByDevice(appId, devid)
			if app != nil {
				pushMessage(appId, app, rawMsg, &msg)
			}
		}
	case PUSH_TYPE_TOPIC: // topic
		apps := AMInstance.GetAppsByTopic(appId, rawMsg.PushParams.Topic)
		for _, app := range apps {
			pushMessage(appId, app, rawMsg, &msg)
		}
	default:
	}
	return nil
}
