package comet

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"github.com/chenyf/push/storage"
	log "github.com/cihub/seelog"
	"sync"
	"time"
)

// register app in storage
type AppInfo struct {
	AppId     string   `json:"app_id"`
	RegTime   int64    `json:"regtime"`
	UserId    string   `json:"uid,omitempty"`
	LastMsgId int64    `json:"last_msgid"`
	Topics    []string `json:"topics"`
	SendIds   []string `json:"sendids,omitempty"`
}

// register app in memory
type RegApp struct {
	DevId string
	RegId string
	AppInfo
}

type AppManager struct {
	lock   *sync.RWMutex
	appMap map[string]*RegApp
}

var (
	AMInstance *AppManager = &AppManager{
		lock:   new(sync.RWMutex),
		appMap: make(map[string]*RegApp),
	}
)

func RegId(devid string, appId string, userId string) string {
	return fmt.Sprintf("%x", (sha1.Sum([]byte(fmt.Sprintf("%s_%s_%s", devid, appId, userId)))))
}

func (this *AppManager) AddApp(devId string, regId string, info *AppInfo) *RegApp {
	this.lock.RLock()
	if app, ok := this.appMap[regId]; ok {
		log.Errorf("%s: regid %s in memory already", devId, regId)
		this.lock.RUnlock()
		return app
	}
	this.lock.RUnlock()
	app := &RegApp{
		DevId: devId,
		RegId: regId,
	}
	app.AppInfo = *info
	this.lock.Lock()
	this.appMap[regId] = app
	this.lock.Unlock()
	return app
}

/*
** delete regapp from app map
 */
func (this *AppManager) DelApp(regId string) {
	this.lock.Lock()
	_, ok := this.appMap[regId]
	if ok {
		delete(this.appMap, regId)
	}
	this.lock.Unlock()
}

/*
** APP注册
** 1. 如果已经在内存中，则失败；不允许多次注册
** 2. 尝试从storage读取app info，如果有的话，则记录到内存中，并返回
** 3. 如果storage中没有，则创建新的结构，保存到storage中，然后记录到内存中，返回
**
 */
func (this *AppManager) RegisterApp(devId string, regId string, appId string, userId string) *RegApp {
	this.lock.RLock()
	if _, ok := this.appMap[regId]; ok {
		log.Errorf("%s: regid %s in memory already", devId, regId)
		this.lock.RUnlock()
		return nil
	}
	this.lock.RUnlock()

	var info AppInfo
	info.AppId = appId
	info.RegTime = time.Now().Unix()
	info.UserId = userId
	info.LastMsgId = -1
	b, _ := json.Marshal(&info)

	if _, err := storage.Instance.HashSet(fmt.Sprintf("db_app_%s", appId), regId, b); err != nil {
		return nil
	}
	// 记录这个设备上有哪些app
	storage.Instance.HashSet(fmt.Sprintf("db_device_%s", devId), appId, []byte(regId))
	// 记录这个用户有哪些app
	if userId != "" {
		storage.Instance.HashSetNotExist(fmt.Sprintf("db_user_%s", userId), regId, []byte(appId))
	}

	regapp := this.AddApp(devId, regId, &info)
	return regapp
}

func (this *AppManager) RegisterApp2(devId string, regId string, appId string, userId string) *RegApp {
	this.lock.RLock()
	if _, ok := this.appMap[regId]; ok {
		log.Errorf("%s: regid %s in memory already", devId, regId)
		this.lock.RUnlock()
		return nil
	}
	this.lock.RUnlock()

	key := fmt.Sprintf("db_app_%s", appId)
	var info AppInfo
	val, err := storage.Instance.HashGet(key, regId)
	if err != nil {
		return nil
	}
	if val == nil {
		return nil
	}
	if err := json.Unmarshal(val, &info); err != nil {
		log.Warnf("invalid app info from storage")
		//TODO need replace it
		return nil
	}
	regapp := this.AddApp(devId, regId, &info)
	return regapp
}

/*
** 从内存中删除结构
 */
func (this *AppManager) UnregisterApp(devId string, regId string, appId string, userId string) {
	if regId == "" {
		return
	}
	// the regapp is still there
	storage.Instance.HashDel(fmt.Sprintf("db_app_%s", appId), regId)
	storage.Instance.HashDel(fmt.Sprintf("db_device_%s", devId), appId)
	if userId != "" {
		storage.Instance.HashDel(fmt.Sprintf("db_user_%s", userId), regId)
	}
	this.DelApp(regId)
}

/*
** update app info into storage
 */
func (this *AppManager) UpdateAppInfo(devId string, regId string, info *AppInfo) bool {
	b, _ := json.Marshal(info)
	if _, err := storage.Instance.HashSet(fmt.Sprintf("db_app_%s", info.AppId), regId, b); err != nil {
		log.Warnf("%s: update app failed, (%s)", devId, err)
		return false
	}
	return true
}

func (this *AppManager) GetApp(appId string, regId string) *RegApp {
	this.lock.RLock()
	regapp, ok := this.appMap[regId]
	if ok {
		this.lock.RUnlock()
		if regapp.AppId != appId {
			return nil
		}
		return regapp
	}
	this.lock.RUnlock()
	return nil
}

func (this *AppManager) GetAppByDevice(appId string, devId string) *RegApp {
	x := DevicesMap.Get(devId)
	if x == nil {
		return nil
	}
	client := x.(*Client)
	for _, regapp := range client.RegApps {
		if regapp.AppId == appId {
			return regapp
		}
	}
	return nil
}

func (this *AppManager) GetApps(appId string) []*RegApp {
	regapps := make([]*RegApp, 0, len(this.appMap))
	this.lock.RLock()
	for _, regapp := range this.appMap {
		if regapp.AppId == appId {
			regapps = append(regapps, regapp)
		}
	}
	this.lock.RUnlock()
	return regapps
}

func (this *AppManager) GetAppsByUser(appId string, userId string) []*RegApp {
	vals, err := storage.Instance.HashGetAll(fmt.Sprintf("db_user_%s", userId))
	if err != nil {
		return nil
	}
	regapps := make([]*RegApp, 0, len(this.appMap))
	this.lock.RLock()
	for index := 0; index < len(vals); index += 2 {
		regid := vals[index]
		appid := vals[index+1]
		if appid != appId {
			continue
		}
		regapp, ok := this.appMap[regid]
		if ok {
			regapps = append(regapps, regapp)
		}
	}
	this.lock.RUnlock()
	return regapps
}

func matchTopics(subcriptions []string, topics []string, topicOp string) bool {
	switch topicOp {
	case "and":
		for _, topic := range topics {
			match := false
			for _, sub := range subcriptions {
				if sub == topic {
					match = true
					break
				}
			}
			if !match {
				return false
			}
		}
		return true
	case "except":
		for i, topic := range topics {
			match := false
			for _, sub := range subcriptions {
				if sub == topic {
					match = true
					break
				}
			}
			if (i == 0 && !match) || (i > 0 && match) {
				return false
			}
		}
		return true
	default: // "or"
		for _, sub := range subcriptions {
			for _, topic := range topics {
				if sub == topic {
					return true
				}
			}
		}
	}
	return false
}

func (this *AppManager) GetAppsByTopic(appId string, topics []string, topicOp string) []*RegApp {
	regapps := make([]*RegApp, 0, len(this.appMap))
	this.lock.RLock()
	for _, regapp := range this.appMap {
		if regapp.AppId != appId {
			continue
		}
		if matchTopics(regapp.Topics, topics, topicOp) {
			regapps = append(regapps, regapp)
		}
	}
	this.lock.RUnlock()
	return regapps
}

func (this *AppManager) LoadAppInfosByDevice(devId string) map[string]*AppInfo {
	vals, err := storage.Instance.HashGetAll(fmt.Sprintf("db_device_%s", devId))
	if err != nil {
		return nil
	}
	infos := make(map[string]*AppInfo)
	for index := 0; index < len(vals); index += 2 {
		appid := vals[index]
		regid := vals[index+1]

		val, err := storage.Instance.HashGet(fmt.Sprintf("db_app_%s", appid), regid)
		if err == nil && val != nil {
			var info AppInfo
			if err := json.Unmarshal(val, &info); err != nil {
				log.Warnf("invalid app info from storage")
				//TODO delete it???
				continue
			}
			infos[regid] = &info
		}
	}
	return infos
}
