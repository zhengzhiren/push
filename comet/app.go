package comet

import (
	//"fmt"
	"log"
)

func PushOutMessage(appId string, pushType int, recvUsers string, msg []byte) {
	// get current online apps
	switch pushType {
	case 0: //broadcast
		apps := AMInstance.GetApps(appId)
		for _, app := range(apps) {
			client := DevicesMap.Get(app.DevId).(*Client)
			if client != nil {
				log.Printf("push to (%s) (%s)", app.DevId, app.RegId)
				client.SendMessage(MSG_PUSH, msg, nil)
			}
		}
	case 1: //regid
		app := AMInstance.Get(appId, recvUsers)
		if app != nil {
			client := DevicesMap.Get(app.DevId).(*Client)
			if client != nil {
				log.Printf("push to (%s) (%s)", app.DevId, recvUsers)
				client.SendMessage(MSG_PUSH, msg, nil)
			}
		}
	case 2:	//alias
	case 3: //tag list
	default:
	}
}

