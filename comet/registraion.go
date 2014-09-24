package comet

import (
	"fmt"
	"github.com/chenyf/push/utils/safemap"
	"github.com/deckarep/golang-set"
)

type App struct {
	DevId	string
	AppId	string
	AppKey	string
	LastMsgSeq	int64
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

func (this *AppManager)RegisterApp(devid string, appid string, appkey string, regid string) {
	app := &App{
		DevId : devid,
		AppId : appid,
		AppKey : appkey,
	}
	this.regMap.Set(regid, app)
	set := this.appMap.Get(appid).(*mapset.Set)
	(*set).Add(regid)
}

func (this *AppManager)UnregisterApp(devid string, appid string, appkey string, regid string) {
	this.regMap.Delete(regid)
	set := this.appMap.Get(appid).(*mapset.Set)
	(*set).Remove(regid)
}

func (this *AppManager)Get(regid string) *App {
	app := this.regMap.Get(regid).(*App)
	return app
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
	set := this.appMap.Get(appId).(*mapset.Set)
	return set
}
