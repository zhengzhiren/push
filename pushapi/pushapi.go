package main

import (
	"os"
	log "github.com/cihub/seelog"
	"os/signal"
	"syscall"
	"net/http"
	"fmt"
	"time"
	"sync"
	"encoding/json"
	"strconv"
	"strings"
	uuid "github.com/codeskyblue/go-uuid"
	"github.com/chenyf/push/conf"
	"github.com/chenyf/push/auth"
	"github.com/chenyf/push/storage"
	"github.com/chenyf/push/mq"
	"github.com/chenyf/push/zk"
)

const (
	TimeToLive = "ttl"
	MsgID = "msgid"
	PappID = "pappid" //appid prefix
	MaxMsgCount = 9223372036854775807
)

const (
	ERR_INTERNAL				= 1000
	ERR_METHOD_NOT_ALLOWED		= 1001
	ERR_BAD_REQUEST             = 1002
	ERR_INVALID_PARAMS          = 1003
	ERR_AUTHENTICATE			= 1004
	ERR_AUTHORIZE				= 1005
	ERR_PKG_EXIST				= 2001
)

type Response struct {
	ErrNo	int				`json:"errno"`
	ErrMsg	string			`json:"errmsg,omitempty"`
	Data	interface{}		`json:"data,omitempty"`
}

var msgBox = make(chan storage.RawMessage, 10)

func checkMessage(m *storage.RawMessage) (bool, string) {
	if m.AppId == "" {
		return false, "missing 'appid'"
	}
	if m.Content == "" {
		return false, "missing 'content'"
	}
	if m.MsgType < 1 || m.MsgType > 2 {
		return false, "invalid 'msg_type'"
	}
	if m.PushType < 1 || m.PushType > 5 {
		return false, "invalid 'push_type'"
	}
	return true, ""
}

func setPappID() error {
	if _, err := storage.Instance.SetNotExist(PappID, []byte("0")); err != nil {
		log.Infof("failed to set AppID prefix: %s", err)
		return err
	}
	return nil
}

func getPappID() int64 {
	if n, err := storage.Instance.IncrBy(PappID, 1); err != nil {
		log.Infof("failed to incr AppID prefix", err)
		return 0
	} else {
		return n
	}
}

func setPackage(uid string, appid string, pkg string) error {
	rawapp := storage.RawApp{
		Pkg : pkg,
		UserId : uid,
	}
	b, _ := json.Marshal(rawapp)
	if _, err := storage.Instance.HashSet("db_apps", appid, b); err != nil {
		log.Infof("failed to set 'db_apps': %s", err)
		return err
	}
	if _, err := storage.Instance.SetAdd("db_pkg_names", pkg); err != nil {
		log.Infof("failed to set 'db_pkg_names': %s", err)
		return err
	}
	return nil
}

func serverHandler(w http.ResponseWriter, r *http.Request) {
	var response Response
	if r.Method != "GET" {
		response.ErrNo = ERR_METHOD_NOT_ALLOWED
		response.ErrMsg = "Method not allowed"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 405)
		return
	}
	node := zk.GetComet()
	if node == nil {
		node = []string{}
		/*
		response.ErrNo = ERR_NO_SERVER
		response.ErrMsg = "no active servers"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 500)
		//fmt.Fprintf(w, string(b))
		return */
	}
	response.ErrNo = 0
	//response.ErrMsg = ""
	response.Data = map[string][]string{"servers": node}
	b, _ := json.Marshal(response)
	fmt.Fprintf(w, string(b))
}

func testHandler(w http.ResponseWriter, r *http.Request) {
	var response Response
	if r.Method != "GET" {
		response.ErrNo = ERR_METHOD_NOT_ALLOWED
		response.ErrMsg = "Method not allowed"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 405)
		return
	}
	response.ErrNo = 0
	b, _ := json.Marshal(response)
	fmt.Fprintf(w, string(b))
}

func appHandler(w http.ResponseWriter, r *http.Request) {
	var response Response
	switch r.Method {
		case "POST":
			addApp(w, r)
			return
		case "DELETE":
			delApp(w, r)
			return
		default:
	}
	response.ErrNo = ERR_METHOD_NOT_ALLOWED
	response.ErrMsg = "Method not allowed"
	b, _ := json.Marshal(response)
	http.Error(w, string(b), 405)
	return
}

func addApp(w http.ResponseWriter, r *http.Request) {
	var response Response
	var data map[string]string
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		response.ErrNo = ERR_BAD_REQUEST
		response.ErrMsg = "Bad request"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 400)
		return
	}
	pkg, pkg_ok := data["pkg"]
	if !pkg_ok {
		response.ErrNo  = ERR_INVALID_PARAMS
		response.ErrMsg = "missing 'pkg'"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 400)
		return
	}
	var uid string
	var ok bool
	uid, uid_ok := data["userid"]
	if !uid_ok {
		token, tk_ok := data["token"]
		if !tk_ok {
			response.ErrNo  = ERR_INVALID_PARAMS
			response.ErrMsg = "missing 'uid' or 'token'"
			b, _ := json.Marshal(response)
			http.Error(w, string(b), 400)
			return
		}
		ok, uid = auth.Instance.Auth(token)
		if !ok {
			response.ErrNo  = ERR_AUTHENTICATE
			response.ErrMsg = "authenticate failed"
			b, _ := json.Marshal(response)
			http.Error(w, string(b), 401)
			return
		}
	}
	tprefix := getPappID()
	if tprefix == 0 {
		response.ErrNo = ERR_INTERNAL
		response.ErrMsg = "no avaiabled appid"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 500)
		return
	}
	n, err := storage.Instance.SetIsMember("db_pkg_names", pkg)
	if err != nil {
		response.ErrNo = ERR_INTERNAL
		response.ErrMsg = "storage I/O failed"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 500)
		return
	}
	if n > 0 {
		response.ErrNo = ERR_PKG_EXIST
		response.ErrMsg = "package exist"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 400)
		return
	}

	prefix := strconv.FormatInt(tprefix, 10)
	tappid := strings.Replace(uuid.New(), "-", "", -1)
	//log.Infof("appid [%s], prefix [%s]", tappid, prefix)
	appid := tappid[0:(len(tappid)-len(prefix))] + prefix
	//log.Infof("appid [%s]", appid)
	if err := setPackage(uid, appid, pkg); err != nil {
		response.ErrNo = ERR_INTERNAL
		response.ErrMsg = "storage I/O failed"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 500)
		return
	}
	response.ErrNo = 0
	response.Data = map[string]string{"appid": appid}
	b, _ := json.Marshal(response)
	fmt.Fprintf(w, string(b))
}

func delApp(w http.ResponseWriter, r *http.Request) {
	var response Response
	var data map[string]string
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		response.ErrNo = ERR_BAD_REQUEST
		response.ErrMsg = "Bad request"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 400)
		return
	}
	pkg, ok1 := data["pkg"]
	appid, ok2 := data["appid"]
	if !ok1 || !ok2 {
		response.ErrNo  = ERR_INVALID_PARAMS
		response.ErrMsg = "missing 'pkg' or 'appid'"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 400)
		return
	}
	var ok bool
	var uid string
	uid, ok = data["userid"]
	if !ok {
		token, ok := data["token"]
		if !ok {
			response.ErrNo  = ERR_INVALID_PARAMS
			response.ErrMsg = "missing 'uid' or 'token'"
			b, _ := json.Marshal(response)
			http.Error(w, string(b), 400)
			return
		}
		ok, uid = auth.Instance.Auth(token)
		if !ok {
			response.ErrNo  = ERR_AUTHENTICATE
			response.ErrMsg = "authenticate failed"
			b, _ := json.Marshal(response)
			http.Error(w, string(b), 401)
			return
		}
	}
	b, err := storage.Instance.HashGet("db_apps", appid)
	if err != nil {
		response.ErrNo  = ERR_INTERNAL
		response.ErrMsg = "storage I/O failed"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 500)
		return
	}
	var rawapp storage.RawApp
	json.Unmarshal(b, &rawapp)
	if rawapp.UserId != uid {
		response.ErrNo  = ERR_AUTHORIZE
		response.ErrMsg = "authorize failed"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 400)
		return
	}
	if _, err := storage.Instance.HashDel("db_apps", appid); err != nil {
		response.ErrNo  = ERR_INTERNAL
		response.ErrMsg = "storage I/O failed"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 500)
		return
	}
	if _, err := storage.Instance.SetDel("db_pkg_names", pkg); err != nil {
		response.ErrNo  = ERR_INTERNAL
		response.ErrMsg = "storage I/O failed"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 500)
		return
	}
	response.ErrNo = 0
	b, _ = json.Marshal(response)
	fmt.Fprintf(w, string(b))
}

func messageHandler(w http.ResponseWriter, r *http.Request) {
	var response Response
	if r.Method != "POST" {
		response.ErrNo = ERR_METHOD_NOT_ALLOWED
		response.ErrMsg = "Method not allowed"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 405)
		return
	}
	msg := storage.RawMessage{}
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		response.ErrNo = ERR_BAD_REQUEST
		response.ErrMsg = "Bad request"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 400)
		return
	}
	var ok bool
	ok, desc := checkMessage(&msg)
	if !ok {
		response.ErrNo  = ERR_INVALID_PARAMS
		response.ErrMsg = desc
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 400)
		return
	}
	uid := msg.UserId
	if uid == "" {
		if msg.Token == "" {
			response.ErrNo  = ERR_INVALID_PARAMS
			response.ErrMsg = "missing 'token' or 'userid'"
			b, _ := json.Marshal(response)
			http.Error(w, string(b), 400)
			return
		}
		ok, uid = auth.Instance.Auth(msg.Token)
		if !ok {
			response.ErrNo  = ERR_AUTHENTICATE
			response.ErrMsg = "authenticate failed"
			b, _ := json.Marshal(response)
			http.Error(w, string(b), 401)
			return
		}
	}
	log.Infof("uid: (%s)", uid)
	b, err := storage.Instance.HashGet("db_apps", msg.AppId)
	//pkg, err := storage.Instance.HashGet(msg.UserId, msg.AppId)
	if err != nil {
		response.ErrNo  = ERR_INTERNAL
		response.ErrMsg = "storage I/O failed"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 500)
		return
	}
	var rawapp storage.RawApp
	json.Unmarshal(b, &rawapp)
	if rawapp.UserId != uid {
		response.ErrNo  = ERR_AUTHORIZE
		response.ErrMsg = "authorize failed"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 400)
		return
	}

	response.ErrNo = 0
	//response.ErrMsg = ""
	//response.Data = map[string]string{"result": "ok"}
	msg.CTime = time.Now().Unix()
	//msg.Pkg = string(pkg)
	msgBox <- msg
	b, _ = json.Marshal(response)
	fmt.Fprintf(w, string(b))
}

func setMsgID() error {
	if _, err := storage.Instance.SetNotExist(MsgID, []byte("0")); err != nil {
		log.Infof("failed to set MsgID: %s", err)
		return err
	}
	return nil
}

func getMsgID() int64 {
	if n, err := storage.Instance.IncrBy(MsgID, 1); err != nil {
		log.Infof("failed to incr MsgID", err)
		return 0
	} else {
		return n
	}
}

func confirmOne(ack, nack chan uint64) {
	select {
		case tag := <-ack:
			log.Infof("confirmed delivery with delivery tag: %d", tag)
		case tag := <-nack:
			log.Infof("failed delivery of delivery tag: %d", tag)
	}
}

func main() {
	config_file := "./conf/conf.json"
	err := conf.LoadConfig(config_file)
	if err != nil {
		fmt.Printf("LoadConfig (%s) failed: (%s)\n", config_file, err)
		os.Exit(1)
	}
	logger, err := log.LoggerFromConfigAsFile("./conf/log.xml")
	if err != nil {
		fmt.Printf("Load log config failed: (%s)\n", err)
		os.Exit(1)
	}

	log.ReplaceLogger(logger)
	setMsgID()
	setPappID()
	mqProducer, err := mq.NewProducer(
		conf.Config.Rabbit.Uri,
		conf.Config.Rabbit.Exchange,
		conf.Config.Rabbit.ExchangeType,
		conf.Config.Rabbit.Key,
		false)
	if err != nil {
		log.Warnf("new mq produccer failed: %s", err)
		os.Exit(1)
	}
	err = zk.InitWatcher(
		conf.Config.ZooKeeper.Addr,
		conf.Config.ZooKeeper.Timeout*time.Second,
		conf.Config.ZooKeeper.Path)
	if err != nil {
			log.Warnf("init zk watcher failed: %s", err)
		os.Exit(1)
	}

	auth.NewInstance(conf.Config.Auth.Provider)
	waitGroup := &sync.WaitGroup{}
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		sig := <-c
		log.Infof("Received signal '%v', exiting\n", sig)
		close(msgBox)
		mqProducer.Shutdown()
		waitGroup.Done()
	}()

	waitGroup.Add(1)
	go func() {
		http.HandleFunc("/api/v1/message",		messageHandler)
		http.HandleFunc("/api/v1/server",		serverHandler)
		http.HandleFunc("/api/v1/app",			appHandler)
		http.HandleFunc("/test/message/confirm",			testHandler)
		err := http.ListenAndServe(conf.Config.PushAPI, nil)
		if err != nil {
			log.Infof("failed to http listen: (%s)", err)
		}
	}()

	for {
		select {
			case m, ok := <-msgBox:
				if !ok {
					os.Exit(0)
				}

				mid := getMsgID()
				if mid == 0 {
					log.Infof("invaild MsgID")
					continue
				}

				m.MsgId = mid
				log.Infof("msg [%v] %v", mid, m)

				v, err := json.Marshal(m)
				if err != nil {
					log.Infof("failed to encode with Msg:", err)
					continue
				}

				if _, err := storage.Instance.HashSet(m.AppId, strconv.FormatInt(mid, 10), v); err != nil {
					log.Infof("failed to put Msg into redis:", err)
					continue
				}
				var ttl int64 = 86400
				if m.Options.TTL > 0 {
					ttl = m.Options.TTL
				}
				_, err = storage.Instance.HashSet(m.AppId+"_offline", fmt.Sprintf("%v_%v", mid, ttl+m.CTime), v)
				if err != nil {
					log.Infof("failed to put offline Msg into redis:", err)
					continue
				}

				d := map[string]interface{}{
					"appid": m.AppId,
					"msgid": mid,
				}
				data, err := json.Marshal(d)
				if err != nil {
					log.Infof("failed to jsonencode with data:", err)
					continue
				}

				if err := mqProducer.Publish(data); err != nil {
					log.Infof("failed to publish data:", err)
					continue
				}
		}
	}
	waitGroup.Wait()
}

