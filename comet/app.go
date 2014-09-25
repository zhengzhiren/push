package comet

import (
	//"fmt"
	"log"
)

func PushMessage(appId string, pushType int, recvUsers string, msg string) {
	// get current online apps
	set := AMInstance.GetByApp(appId)

	switch pushType {
	case 0: //broadcast
		for item := range((*set).Iter()) {
			regid := item.(string)
			app := AMInstance.Get(regid)
			devid := app.DevId
			if DevicesMap.Check(devid) {
				client := DevicesMap.Get(devid).(*Client)
				log.Printf("push to (%s) (%s)", devid, regid)
				client.SendMessage(MSG_REQUEST, []byte(msg), nil)
			}
		}
	case 1: //regid
		app := AMInstance.Get(recvUsers)
		devid := app.DevId
		if DevicesMap.Check(devid) {
			client := DevicesMap.Get(devid).(*Client)
			log.Printf("push to (%s) (%s)", devid, recvUsers)
			client.SendMessage(MSG_REQUEST, []byte(msg), nil)
		}
	case 2:	//alias
	case 3: //tag list
	default:
	}
}

