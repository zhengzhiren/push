package comet

import (
	"fmt"
	"log"
	"sync"
	//"github.com/chenyf/push/error"
	"github.com/chenyf/push/storage"
)

type App struct {
	RegId		string
	DevId		string
	AppId		string
	LastMsgId	int64
}
type AppManager struct {
	lock *sync.RWMutex
	appMap map[string]*App
}

var (
	AMInstance *AppManager = &AppManager{
		lock: new(sync.RWMutex),
		appMap: make(map[string]*App), 
	}
)

func RegId(devid string, appKey string) string {
	return fmt.Sprintf("%s_%s", devid, appKey)
}

func (this *AppManager)RemoveApp(regId string)  {
	this.lock.Lock()
	log.Printf("remove app (%s)", regId)
	delete(this.appMap, regId)
	this.lock.Unlock()
}

func (this *AppManager)RegisterApp(devId string, appId string, appKey string, regId string) (*App) {
	var last_msgid int64 = -1
	if regId != "" {
		// 非第一次注册
		this.lock.RLock()
		if _, ok := this.appMap[regId]; ok {
			log.Printf("in memory already")
			this.lock.RUnlock()
			return nil
		}
		this.lock.RUnlock()
		// 从后端存储获取 last_msgid
		info := storage.StorageInstance.GetApp(appId, regId)
		if info == nil {
			log.Printf("not in storage")
			return nil
		}
		log.Printf("got last msgid %d", info.LastMsgId)
		last_msgid = info.LastMsgId
	} else {
		// 第一次注册，分配一个新的regId
		regId = RegId(devId, appId)
		// 记录到后端存储中
		if err := storage.StorageInstance.UpdateApp(appId, regId, -1); err != nil {
			log.Printf("storage add failed")
			return nil
		}
	}
	app := &App{
		RegId : regId,
		DevId : devId,
		AppId : appId,
		LastMsgId : last_msgid,
	}
	this.lock.Lock()
	log.Printf("register app (%s) (%s) (%d)", appId, regId, last_msgid)
	this.appMap[regId] = app
	this.lock.Unlock()
	return app
}

func (this *AppManager)UnregisterApp(devId string, appId string, appKey string, regId string) {
	/*
	if this.appMap.Check(regId) {
		this.appMap.Delete(regId)
	}
	*/
}

func (this *AppManager)Get(appId string, regId string) *App {
	this.lock.Lock()
	app, ok := this.appMap[regId]; if ok {
		this.lock.Unlock()
		return app
	}
	this.lock.Unlock()
	return nil
}

/*
func (this *AppManager)Set(appId string, regId string, app *App) {
	this.appMap.Set(regId, app)
}

func (this *AppManager)Check(appId string, regId string) bool {
	return this.appMap.Check(regId)
}

func (this *AppManager)Delete(appId string, regId string) {
	this.appMap.Delete(regId)
}
*/

func (this *AppManager)GetApps(appId string) ([]*App) {
	apps := make([]*App, 0, len(this.appMap))
	this.lock.RLock()
	for _, app := range(this.appMap) {
		if app.AppId == appId {
			apps = append(apps, app)
		}
	}
	this.lock.RUnlock()
	log.Printf("get %d apps", len(apps))
	return apps
}

func (this *AppManager)UpdateApp(appId string, regId string, msgId int64, app *App) error {
	if err := storage.StorageInstance.UpdateApp(appId, regId, msgId); err != nil {
		log.Printf("storage update app failed")
		return err
	}
	app.LastMsgId = msgId
	return nil
}
