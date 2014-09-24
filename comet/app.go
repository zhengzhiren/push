package comet

import (
	//"fmt"
	"github.com/chenyf/push/utils/safemap"
)

type AppItem struct {
	RegList	[]string
}

var (
	AppsMap *safemap.SafeMap = safemap.NewSafeMap()
)

/*
func PushMessage(appKey string, recvType int, recvUsers string, msg string) {
	appItem := AppsMap.Get(appKey)
	select recvType {
	case 1: //broadcast
		for regid := range(appItem.RegList) {
			regItem := RegistrationsMap.Get(regid)
			devid := regItem.DeviceId
			if DevicesMap.Check(devid) {
				device := DevicesMap.Get(devid)
				device.SendMessage(msg)
			}
		}
	case 2: //regid
		regid := RegistrationsMap.Get(recvUsers)
			regItem := RegistrationsMap.Get(regid)
			devid := regItem.DeviceId
			if DevicesMap.Check(devid) {
				device := DevicesMap.Get(devid)
				device.SendMessage(msg)
			}
	case 3:	//alias
	case 4: //tag list
	default:
	}
}
*/

