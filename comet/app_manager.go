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
type AppInfo struct {
	AppId		string	`json:"app_id"`
	UserId		string	`json:"user_id"`
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
func (this *AppManager)RegisterApp(devId string, appId string, regId string, userId string) (*App) {
	this.lock.RLock()
	if _, ok := this.appMap[regId]; ok {
		log.Warnf("in memory already")
		this.lock.RUnlock()
		return nil
	}
	this.lock.RUnlock()

	// 如果已经在后端存储中存在，则获取 last_msgid
	var last_msgid int64 = -1
	val, err := storage.StorageInstance.HashGet(fmt.Sprintf("db_app_%s", appId), regId)
	if err == nil && val != nil {
		var info AppInfo
		if err := json.Unmarshal(val, &info); err != nil {
			log.Warnf("invalid app info from storage")
			return nil
		}
		log.Infof("got last msgid %d", info.LastMsgId)
		last_msgid = info.LastMsgId
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

	// 记录这个设备上有哪些app
	storage.StorageInstance.HashSetNotExist(fmt.Sprintf("db_device_app_%s", devId), regId, []byte(appId))
	// 记录这个用户有哪些app
	if userId != "" {
		storage.StorageInstance.SetAdd(fmt.Sprintf("db_user_app_%s", userId), regId)
	}
	return app
}

func (this *AppManager)UnregisterApp(devId string, appId string, appKey string, regId string, userId string) {
	if regId == "" {
		return
	}
	this.lock.Lock()
	_, ok := this.appMap[regId]
	if ok {
		storage.StorageInstance.HashDel(fmt.Sprintf("db_app_%s", appId), regId)
		storage.StorageInstance.HashDel(fmt.Sprintf("db_device_app_%s", devId), regId)
		if userId != "" {
			storage.StorageInstance.SetMove(fmt.Sprintf("db_user_app_%s", userId), regId)
		}
		delete(this.appMap, regId)
	}
	this.lock.Unlock()
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

func (this *AppManager)LoadAppInfosByDevice(devId string) map[string]*AppInfo {
	vals, err := storage.StorageInstance.HashGetAll(fmt.Sprintf("db_device_app_%s", devId))
	if err != nil {
		return nil
	}
	infos := make(map[string]*AppInfo)
	for index := 0; index < len(vals); index+=2 {
		regid := vals[index]
		appid := vals[index+1]

		val, err := storage.StorageInstance.HashGet(fmt.Sprintf("db_app_%s", appid), regid)
		if err == nil && val != nil {
			var info AppInfo
			if err := json.Unmarshal(val, &info); err != nil {
				log.Warnf("invalid app info from storage")
				continue
			}
			infos[regid] = &info
		}
	}
	return infos
}

