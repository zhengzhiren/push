package comet

import (
	"fmt"
	"github.com/chenyf/push/utils/safemap"
)

type App struct {
	DevId	string
	AppId	string
	AppKey	string
	LastMsgSeq	int64
}

type RegisterManager struct {
	Map *safemap.SafeMap
}

var (
	RegisterManagerInstance *RegisterManager = &RegisterManager{
		Map : safemap.NewSafeMap(),
	}
)

func RegId(devid string, appKey string) string {
	return fmt.Sprintf("%s_%s", devid, appKey)
}

func (this *RegisterManager)Get(regid string) *App {
	app := this.Map.Get(regid).(*App)
	return app
}

func (this *RegisterManager)Set(regid string, app *App) {
	this.Map.Set(regid, app)
}

func (this *RegisterManager)Check(regid string) bool {
	return this.Map.Check(regid)
}

func (this *RegisterManager)Delete(regid string) {
	this.Map.Delete(regid)
}

