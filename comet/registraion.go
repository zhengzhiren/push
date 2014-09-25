package comet

import (
	"fmt"
	"log"
	"github.com/chenyf/push/error"
	"github.com/chenyf/push/storage"
	"github.com/chenyf/push/utils/safemap"
	"github.com/deckarep/golang-set"
)

type App struct {
	DevId		string
	AppId		string
	AppKey		string
	LastMsgId	int64
}
type AppManager struct {
	regMap *safemap.SafeMap
	appMap *safemap.SafeMap
}

var (
	AMInstance *AppManager = &AppManager{
		regMap : safemap.NewSafeMap(),
		appMap : safemap.NewSafeMap(),
	}
)

func RegId(devid string, appKey string) string {
	return fmt.Sprintf("%s_%s", devid, appKey)
}

func (this *AppManager)RegisterApp(devId string, appId string, appKey string, regId string) (string, error) {
	var last_msgid int64 = -1
	if regId == "" {
		// the first time to register, assign new regid
		regId = RegId(devId, appId)
		// store it : app on device
		if err := storage.StorageInstance.AddApp(regId, appId, appKey, devId); err != nil {
			log.Printf("register failed")
			return "", err
		}
	} else  {
		// not the first time
		if this.regMap.Check(regId) {
			log.Printf("register already")
			return regId, nil
		} else {
			// get info from storage
			app_info := storage.StorageInstance.GetApp(regId)
			if app_info == nil {
				return "", &pusherror.PushError{"regid not found in storage"}
			}
			last_msgid = app_info.LastMsgId
		}
	}

	app := &App{
		DevId : devId,
		AppId : appId,
		AppKey : appKey,
		LastMsgId : last_msgid,
	}

	this.regMap.Set(regId, app)
	if this.appMap.Check(appId) {
		set := this.appMap.Get(appId).(mapset.Set)
		set.Add(regId)
	} else {
		set := mapset.NewSet()
		set.Add(regId)
		this.appMap.Set(appId, set)
	}
	return regId, nil
}

func (this *AppManager)UnregisterApp(devid string, appid string, appkey string, regid string) {
	if this.regMap.Check(regid) {
		this.regMap.Delete(regid)
		if this.appMap.Check(appid) {
			set := this.appMap.Get(appid).(mapset.Set)
			set.Remove(regid)
		}
	}
}

func (this *AppManager)Get(regid string) *App {
	if this.regMap.Check(regid) {
		app := this.regMap.Get(regid).(*App)
		return app
	}
	return nil
}

func (this *AppManager)Set(regid string, app *App) {
	this.regMap.Set(regid, app)
}

func (this *AppManager)Check(regid string) bool {
	return this.regMap.Check(regid)
}

func (this *AppManager)Delete(regid string) {
	this.regMap.Delete(regid)
}

func (this AppManager)GetByApp(appId string) (*mapset.Set) {
	if this.appMap.Check(appId) {
		set := this.appMap.Get(appId).(mapset.Set)
		return &set
	}
	return nil
}

