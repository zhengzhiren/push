package comet

import (
	//"fmt"
	"log"
	"encoding/json"
	"github.com/chenyf/push/message"
)

func PushOutMessage(appId string, pushType int, recvUsers string, msg []byte) {
	// get current online apps
	switch pushType {
	case 0: //broadcast
		apps := AMInstance.GetApps(appId)
		for _, app := range(apps) {
			client := DevicesMap.Get(app.DevId).(*Client)
			if client != nil {
				log.Printf("push to (app %s) (device %s) (regid %s)", appId, app.DevId, app.RegId)
				client.SendMessage(MSG_PUSH, msg, nil)
			}
		}
	case 1: //regid
		app := AMInstance.Get(appId, recvUsers)
		if app != nil {
			client := DevicesMap.Get(app.DevId).(*Client)
			if client != nil {
				log.Printf("push to (app %s) (device %s) (regid %s)", appId, app.DevId, recvUsers)
				client.SendMessage(MSG_PUSH, msg, nil)
			}
		}
	case 2:	//alias
	case 3: //tag list
	default:
	}
}

func SimplePushMessage(appId string, rawMsg *message.RawMessage) error {
	msg := message.PushMessage{
		MsgId: rawMsg.MsgId,
		AppId: appId,
		Type: rawMsg.PushType,
		Content: rawMsg.Content, //FIXME
	}
	switch rawMsg.PushType {
		case 1:
			apps := AMInstance.GetApps(appId)
			for _, app := range(apps) {
				client := DevicesMap.Get(app.DevId).(*Client)
				if client != nil {
					log.Printf("push to (app %s) (device %s) (regid %s)", appId, app.DevId, app.RegId)
					fmsg, err := json.Marshal(msg)
					if err != nil {
						log.Printf("failed to encode msg %v", msg)
						continue
					}
					client.SendMessage(MSG_PUSH, fmsg, nil)
				}
			}
		case 2:
		case 3:
		default:
	}
	return nil
}
