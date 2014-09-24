package comet

import (
	//"fmt"
)

func PushMessage(appId string, recvType int, recvUsers string, msg string) {
	set := AMInstance.GetByApp(appId)

	switch recvType {
	case 1: //broadcast
		for item := range((*set).Iter()) {
			regid := item.(string)
			app := AMInstance.Get(regid)
			devid := app.DevId
			if DevicesMap.Check(devid) {
				client := DevicesMap.Get(devid).(*Client)
				client.SendMessage(MSG_REQUEST, []byte(msg), nil)
			}
		}
	case 2: //regid
	case 3:	//alias
	case 4: //tag list
	default:
	}
}

