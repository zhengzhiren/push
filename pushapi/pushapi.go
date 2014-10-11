package main

import (
	"flag"
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
	"github.com/chenyf/push/conf"
	"github.com/chenyf/push/auth"
	"github.com/chenyf/push/storage"
	"github.com/chenyf/push/mq"
	"github.com/chenyf/push/zk"
)

const (
	TimeToLive = "ttl"
	MsgID = "msgid"
	MaxMsgCount = 9223372036854775807
)

type Message struct {
	Token	string			`json:"token"`
	MsgId		int64		`json:"msgid"`
	AppId		string		`json:"appid"`
	CTime		int64		`json:"ctime"`
	Platform	string		`json:"platform"`
	MsgType		int			`json:"msg_type"`
	PushType	int			`json:"push_type"`
	Content		string		`json:"content"`
	Options		interface{}	`json:"options"`
}

type PResponse struct {
	ErrNo	int				`json:"errno"`
	ErrMsg	string			`json:"errmsg"`
}

var (
	zkAddr	     = flag.String("zkaddr", "10.154.156.121:2181", "zookeeper addrs")
	zkTimeout    = flag.Duration("zktimeout", 30, "zookeeper timeout")
	zkPath       = flag.String("zkpath", "/push", "zookeeper path")
)

var msgBox = make(chan Message, 10)

type tokenResult struct {
	bean	struct {
		result	string		`json:"result"`
	}		`json:"bean"`
	status	string			`json:"status"`
	errcode	string			`json:"errorCode"`
}

func checkMessage(m *Message) bool {
	ret := true
	if m.AppId == "" || m.Content == "" || m.MsgType == 0 || m.PushType == 0 {
		ret = false
	}
	return ret
}

func getComet(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method Not Allowed", 405)
		return
	}
	node := zk.GetComet()
	if node == nil {
		http.Error(w, "No active comet", 404)
		return
	}
	fmt.Fprintf(w, node.TcpAddr)
}

func postSendMsg(w http.ResponseWriter, r *http.Request) {
	var response PResponse
	msg := Message{}
	response.ErrNo = 0
	if r.Method != "POST" {
		response.ErrNo  = 1001
		response.ErrMsg = "must using 'POST' method\n"
		b, _ := json.Marshal(response)
		fmt.Fprintf(w, string(b))
		return
	}

	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		response.ErrNo  = 1002
		response.ErrMsg = "invaild POST body"
		b, _ := json.Marshal(response)
		fmt.Fprintf(w, string(b))
		return
	}
	if msg.CTime == 0 {
		msg.CTime = time.Now().Unix()
	}
	//log.Print(msg)
	ok, uid := auth.CheckAuth(msg.Token)
	if !ok {
		response.ErrNo  = 1003
		response.ErrMsg = "auth failed"
		b, _ := json.Marshal(response)
		fmt.Fprintf(w, string(b))
		return
	}
	log.Infof("uid: (%s)", uid)
	if !checkMessage(&msg) {
		response.ErrNo  = 1004
		response.ErrMsg = "invaild Message"
		b, _ := json.Marshal(response)
		fmt.Fprintf(w, string(b))
		return
	}
	uid2, err := storage.StorageInstance.HashGet("db_apps", msg.AppId)
	if err !=nil || uid != string(uid2) {
		response.ErrNo  = 1005
		response.ErrMsg = "user auth failed"
		b, _ := json.Marshal(response)
		fmt.Fprintf(w, string(b))
		return
	}

	response.ErrMsg = ""
	msgBox <- msg
	b, _ := json.Marshal(response)
	fmt.Fprintf(w, string(b))
}

func setMsgID() error {
	if _, err := storage.StorageInstance.SetNotExist(MsgID, []byte("0")); err != nil {
		log.Infof("failed to set MsgID: %s", err)
		return err
	}
	return nil
}

func getMsgID() int64 {
	if n, err := storage.StorageInstance.IncrBy(MsgID, 1); err != nil {
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

	mqProducer, err := mq.NewProducer(
		conf.Config.Rabbit.Uri,
		conf.Config.Rabbit.Exchange,
		conf.Config.Rabbit.ExchangeType,
		conf.Config.Rabbit.Key,
		conf.Config.Rabbit.Reliable)
	if err != nil {
		log.Warnf("%s", err)
		os.Exit(1)
	}
	err = zk.InitZK(*zkAddr, (*zkTimeout)*time.Second, *zkPath)
	if err != nil {
		log.Warnf("%s", err)
		os.Exit(1)
	}
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
		http.HandleFunc("/v1/comet", getComet)
		err := http.ListenAndServe("0.0.0.0:8080", nil)
		if err != nil {
			log.Infof("failed to http listen:", err)
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

				if _, err := storage.StorageInstance.HashSet(m.AppId, strconv.FormatInt(mid, 10), v); err != nil {
					log.Infof("failed to put Msg into redis:", err)
					continue
				}
				if m.Options != nil {
					ttl, ok := m.Options.(map[string]interface{})[TimeToLive]
					if ok && int64(ttl.(float64)) > 0 {
						if _, err := storage.StorageInstance.HashSet(
								m.AppId+"_offline",
								fmt.Sprintf("%v_%v",
								mid, int64(ttl.(float64))+m.CTime), v); err != nil {
							log.Infof("failed to put offline Msg into redis:", err)
							continue
						}
					}
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

