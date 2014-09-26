package comet

import (
	"fmt"
	"log"
	"github.com/chenyf/push/error"
	"github.com/chenyf/push/storage"
	"github.com/chenyf/push/utils/safemap"
)

type App struct {
	RegId		string
	DevId		string
	LastMsgId	int64
}
type AppManager struct {
	appMap *safemap.SafeMap
}

var (
	AMInstance *AppManager = &AppManager{
		appMap : safemap.NewSafeMap(),
	}
)

func RegId(devid string, appKey string) string {
	return fmt.Sprintf("%s_%s", devid, appKey)
}

func (this *AppManager)RegisterApp(devId string, appId string, appKey string, regId string) (string, error) {
	var last_msgid int64 = -1
	if regId != "" {
		// 非第一次注册
		key := fmt.Sprintf("%s_%s", appId, regId)
		if this.appMap.Check(key) {
			// 已在内存中
			return regId, nil
		}
		// 从后端存储获取 last_msgid
		app_info := storage.StorageInstance.GetApp(appId, regId)
		if app_info == nil {
			return "", &pusherror.PushError{"regid not found in storage"}
		}
		last_msgid = app_info.LastMsgId
	} else {
		// 第一次注册，分配一个新的regId
		regId = RegId(devId, appId)
		// 记录到后端存储中
		if err := storage.StorageInstance.AddApp(appId, regId, appKey, devId); err != nil {
			log.Printf("storage add failed")
			return "", err
		}
	}
	key := fmt.Sprintf("%s_%s", appId, regId)
	app := &App{
		RegId : regId,
		DevId : devId,
		LastMsgId : last_msgid,
	}
	this.appMap.Set(key, app)
	return regId, nil
}

func (this *AppManager)UnregisterApp(devId string, appId string, appKey string, regId string) {
	key := fmt.Sprintf("%s_%s", appId, regId)
	if this.appMap.Check(key) {
		this.appMap.Delete(key)
	}
}

func (this *AppManager)Get(appId string, regId string) *App {
	key := fmt.Sprintf("%s_%s", appId, regId)
	if this.appMap.Check(key) {
		app := this.appMap.Get(key).(*App)
		return app
	}
	return nil
}

func (this *AppManager)Set(appId string, regId string, app *App) {
	key := fmt.Sprintf("%s_%s", appId, regId)
	this.appMap.Set(key, app)
}

func (this *AppManager)Check(appId string, regId string) bool {
	key := fmt.Sprintf("%s_%s", appId, regId)
	return this.appMap.Check(key)
}

func (this *AppManager)Delete(appId string, regId string) {
	key := fmt.Sprintf("%s_%s", appId, regId)
	this.appMap.Delete(key)
}

func (this *AppManager)GetApps(appId string) ([]*App) {
	return nil
}

