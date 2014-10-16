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

type Response struct {
	ErrNo	int			`json:"errno"`
	ErrMsg	string			`json:"errmsg"`
	Data	interface{}		`json:"data,omitempty"`
}

var msgBox = make(chan storage.RawMessage, 10)

func checkMessage(m *storage.RawMessage) bool {
	ret := true
	if m.AppId == "" || m.Content == "" || m.MsgType == 0 || m.PushType == 0 {
		ret = false
	}
	return ret
}

func getPushServer(w http.ResponseWriter, r *http.Request) {
	var response Response
	response.ErrNo = 0
	response.ErrMsg = ""

	if r.Method != "GET" {
		http.Error(w, "Method Not Allowed", 405)
		return
	}
	node := zk.GetComet()
	if node == nil {
		response.ErrNo = 1031
		response.ErrMsg = "no active servers"
		b, _ := json.Marshal(response)
		fmt.Fprintf(w, string(b))
		return
	}
	response.Data = map[string][]string{"servers": node}
	b, _ := json.Marshal(response)
	fmt.Fprintf(w, string(b))
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

func setPackage(uid string, pkg string, appid string) error {
	if _, err := storage.Instance.HashSet(uid, appid, []byte(pkg)); err != nil {
		log.Infof("failed to set Package: %s", err)
		return err
	}
	return nil
}

func postGenId(w http.ResponseWriter, r *http.Request) {
	var response Response
	response.ErrNo = 0
	response.ErrMsg = ""
	if r.Method != "POST" {
		http.Error(w, "Method Not Allowed", 405)
		return
	}
	var data map[string]string
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Bad Request", 400)
		return
	}
	sso_tk, tk_ok := data["sso_tk"]
	pkg, pkg_ok := data["package"]
	if !tk_ok || !pkg_ok {
		response.ErrNo = 1061
		response.ErrMsg = "invalid args"
		b, _ := json.Marshal(response)
		fmt.Fprintf(w, string(b))
		return
	}

	ok, uid := auth.Instance.Auth(sso_tk)
	if !ok {
		http.Error(w, "Unauthorized", 401)
		return
	}
	tprefix := getPappID()
	if tprefix == 0 {
		response.ErrNo = 1061
		response.ErrMsg = "no availed appid"
		b, _ := json.Marshal(response)
		fmt.Fprintf(w, string(b))
		return
	}
	prefix := strconv.FormatInt(tprefix, 10)
	tappid := strings.Replace(uuid.New(), "-", "", -1)
	//log.Infof("appid [%s], prefix [%s]", tappid, prefix)
	appid := tappid[0:(len(tappid)-len(prefix))] + prefix
	//log.Infof("appid [%s]", appid)
	if err := setPackage(uid, appid, pkg); err != nil {
		response.ErrNo = 2002
		response.ErrMsg = "failed to store package"
		b, _ := json.Marshal(response)
		fmt.Fprintf(w, string(b))
		return
	}
	response.Data = map[string]string{"appid": appid}
	b, _ := json.Marshal(response)
	fmt.Fprintf(w, string(b))
}

func postSendMsg(w http.ResponseWriter, r *http.Request) {
	var response Response
	msg := storage.RawMessage{}
	response.ErrNo = 0
	response.ErrMsg = ""
	if r.Method != "POST" {
		http.Error(w, "Method Not Allowed", 405)
		return
	}
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		response.ErrNo  = 1001
		response.ErrMsg = "invalid POST body"
		b, _ := json.Marshal(response)
		fmt.Fprintf(w, string(b))
		return
	}
	msg.CTime = time.Now().Unix()
	ok, uid := auth.Instance.Auth(msg.Token)
	if !ok {
		response.ErrNo  = 1002
		response.ErrMsg = "auth failed"
		b, _ := json.Marshal(response)
		fmt.Fprintf(w, string(b))
		return
	}
	log.Infof("uid: (%s)", uid)
	if !checkMessage(&msg) {
		response.ErrNo  = 1003
		response.ErrMsg = "invaild Message"
		b, _ := json.Marshal(response)
		fmt.Fprintf(w, string(b))
		return
	}
	flag := false
	vals, _ := storage.Instance.HashGetAll(fmt.Sprintf("db_user_%s", uid))
	for index := 0; index < len(vals); index+=2 {
		appid := vals[index+1]
		if appid == msg.AppId {
			flag = true
			break
		}
	}
	if !flag {
		response.ErrNo  = 1004
		response.ErrMsg = "user auth failed"
		b, _ := json.Marshal(response)
		fmt.Fprintf(w, string(b))
		return
	}

	response.Data = map[string]string{"result": "ok"}
	msgBox <- msg
	b, _ := json.Marshal(response)
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
		http.HandleFunc("/v1/push/message", postSendMsg)
		http.HandleFunc("/v1/push/server", getPushServer)
		http.HandleFunc("/v1/push/appid/gen", postGenId)
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

