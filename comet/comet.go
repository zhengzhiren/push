package comet

import (
	//"fmt"
	"encoding/json"
	"github.com/chenyf/push/storage"
	log "github.com/cihub/seelog"
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

func pushMessage(appId string, app *RegApp, msg *PushMessage) bool {
	client := DevicesMap.Get(app.DevId).(*Client)
	if client == nil {
		return false
	}
	log.Infof("push to (app %s) (device %s) (regid %s)", appId, app.DevId, app.RegId)
	b, err := json.Marshal(msg)
	if err != nil {
		log.Infof("failed to encode msg %v", msg)
		return false
	}
	client.SendMessage(MSG_PUSH, 0, b, nil)
	return true
}

func PushMessages(appId string, rawMsg *storage.RawMessage) error {
	msg := PushMessage{
		MsgId:   rawMsg.MsgId,
		AppId:   appId,
		Type:    rawMsg.MsgType,
		Content: rawMsg.Content, //FIXME
	}
	switch rawMsg.PushType {
	case 1: // broadcast
		apps := AMInstance.GetApps(appId)
		for _, app := range apps {
			pushMessage(appId, app, &msg)
		}
	case 2: // regid list
		for _, regid := range rawMsg.PushParams.RegId {
			app := AMInstance.GetApp(appId, regid)
			if app != nil {
				pushMessage(appId, app, &msg)
			}
		}
	case 3: // userid list
		for _, uid := range rawMsg.PushParams.UserId {
			apps := AMInstance.GetAppsByUser(appId, uid)
			for _, app := range apps {
				pushMessage(appId, app, &msg)
			}
		}
	case 4: // devid list
		for _, devid := range rawMsg.PushParams.DevId {
			app := AMInstance.GetAppByDevice(appId, devid)
			if app != nil {
				pushMessage(appId, app, &msg)
			}
		}
	case 5: // topic
		apps := AMInstance.GetAppsByTopic(appId, rawMsg.PushParams.Topic)
		for _, app := range apps {
			pushMessage(appId, app, &msg)
		}
	default:
	}
	return nil
}
