package comet

import (
	"fmt"
	"sync"
	"encoding/json"
	log "github.com/cihub/seelog"
	"github.com/chenyf/push/storage"
)

type App struct {
	RegId		string
	DevId		string
	AppId		string
	LastMsgId	int64
}

// struct will save into storage
type AppInfo struct{
	LastMsgId	int64	`json:"last_msgid"`
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
	//log.Infof("remove app (%s)", regId)
	delete(this.appMap, regId)
	this.lock.Unlock()
}

// APP注册
func (this *AppManager)RegisterApp(devId string, appId string, appKey string, regId string) (*App) {
	var last_msgid int64 = -1
	if regId != "" {
		// 非第一次注册
		this.lock.RLock()
		if _, ok := this.appMap[regId]; ok {
			log.Warnf("in memory already")
			this.lock.RUnlock()
			return nil
		}
		this.lock.RUnlock()
		// 从后端存储获取 last_msgid
		val, err := storage.StorageInstance.HashGet(fmt.Sprintf("db_app_%s", appId), regId)
		if err != nil {
			return nil
		}
		if val == nil {
			log.Warnf("not found regid from storage")
			return nil
		}
		var info AppInfo
		if err := json.Unmarshal(val, &info); err != nil {
			log.Warnf("invalid app info from storage")
			return nil
		}
		log.Infof("got last msgid %d", info.LastMsgId)
		last_msgid = info.LastMsgId
	} else {
		// 第一次注册，分配一个新的regId
		regId = RegId(devId, appId)
		info := AppInfo{
			LastMsgId : -1,
		}
		val, _ := json.Marshal(info)
		// 记录到后端存储中
		if _, err := storage.StorageInstance.HashSet(fmt.Sprintf("db_app_%s", appId), regId, val); err != nil {
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
	log.Infof("register app (%s) (%s) (%d)", appId, regId, last_msgid)
	this.appMap[regId] = app
	this.lock.Unlock()
	return app
}

func (this *AppManager)UnregisterApp(devId string, appId string, appKey string, regId string) {
	if regId == "" {
		return
	}
	this.lock.Lock()
	_, ok := this.appMap[regId]
	if ok {
		storage.StorageInstance.HashDel(fmt.Sprintf("db_app_%s", appId), regId)
		delete(this.appMap, regId)
	}
	this.lock.Unlock()
}

func (this *AppManager)GetApp(appId string, regId string) *App {
	this.lock.RLock()
	app, ok := this.appMap[regId]; if ok {
		this.lock.RUnlock()
		return app
	}
	this.lock.RUnlock()
	return nil
}

func (this *AppManager)GetApps(appId string) ([]*App) {
	apps := make([]*App, 0, len(this.appMap))
	this.lock.RLock()
	for _, app := range(this.appMap) {
		if app.AppId == appId {
			apps = append(apps, app)
		}
	}
	this.lock.RUnlock()
	log.Infof("get %d apps", len(apps))
	return apps
}

func (this *AppManager)UpdateApp(appId string, regId string, msgId int64, app *App) error {
	info := AppInfo{
		LastMsgId : msgId,
	}
	b, _ := json.Marshal(info)
	if _, err := storage.StorageInstance.HashSet(fmt.Sprintf("db_app_%s", appId), regId, b); err != nil {
		return err
	}
	storage.StorageInstance.HashIncrBy("db_msg_stat", fmt.Sprintf("%d", msgId), 1)
	app.LastMsgId = msgId
	return nil
}

