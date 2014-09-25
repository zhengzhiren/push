package comet

import (
	//"fmt"
	"log"
)

func PushOutMessage(appId string, pushType int, recvUsers string, msg []byte) {
	// get current online apps
	set := AMInstance.GetByApp(appId)
	if set == nil {
		return
	}

	switch pushType {
	case 0: //broadcast
		for item := range((*set).Iter()) {
			regid := item.(string)
			app := AMInstance.Get(regid)
			devid := app.DevId
			if DevicesMap.Check(devid) {
				log.Printf("push to (%s) (%s)", devid, regid)
				client := DevicesMap.Get(devid).(*Client)
				client.SendMessage(MSG_PUSH, msg, nil)
			}
		}
	case 1: //regid
		app := AMInstance.Get(recvUsers)
		devid := app.DevId
		if DevicesMap.Check(devid) {
			client := DevicesMap.Get(devid).(*Client)
			log.Printf("push to (%s) (%s)", devid, recvUsers)
			client.SendMessage(MSG_PUSH, msg, nil)
		}
	case 2:	//alias
	case 3: //tag list
	default:
	}
}
