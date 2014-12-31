package main

import (
	"flag"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"encoding/json"
	"fmt"
	"time"
	"bytes"
	"io/ioutil"

	log "github.com/cihub/seelog"
	"github.com/chenyf/push/utils"
	"github.com/chenyf/push/conf"
)

const (
	ERR_INTERNAL           = 1000
	ERR_METHOD_NOT_ALLOWED = 1001
	ERR_BAD_REQUEST        = 1002
	ERR_INVALID_PARAMS     = 1003
	ERR_AUTHENTICATE       = 1004
	ERR_AUTHORIZE          = 1005
	ERR_SIGN               = 1006
	ERR_EXIST              = 2001
	ERR_NOT_EXIST          = 2002
)

type Response struct {
	ErrNo  int         `json:"errno"`
	ErrMsg string      `json:"errmsg,omitempty"`
	Data   interface{} `json:"data,omitempty"`
}

type SyncMsg struct {
	AppId	string		`json:"appid"`
	UserId	string		`json:"uid"`
	SendId	string		`json:"sendid"`
	RegId	string		`json:"regid"`
	//DevId	string		`json:"devid"`
}

type PushMsg struct {
	AppId      string `json:"appid"`
	MsgType    int    `json:"msg_type"`
	PushType   int    `json:"push_type"`
	PushParams struct {
		UserId []string `json:"userid,omitempty"`
	} `json:"push_params"`
	Content      string `json:"content,omitempty"`
	Options struct {
		TTL int64 `json:"ttl,omitempty"`
	} `json:"options"`
	SendId string `json:"sendid,omitempty"`
}

var msgBox = make(chan *SyncMsg, 100)

func errResponse(w http.ResponseWriter, errno int, errmsg string, httpcode int) {
	var response Response
	response.ErrNo = errno
	response.ErrMsg = errmsg
	b, _ := json.Marshal(response)
	http.Error(w, string(b), httpcode)
}

func push(syncmsg *SyncMsg) (*http.Response, error) {
	pushurl := "http://push.scloud.letv.com/api/v1/message"
	pushmsg := &PushMsg{
		AppId : syncmsg.AppId,
		MsgType : 2,
		PushType : 3,
		Content : fmt.Sprintf("{\"sendid\" : \"%s\"}", syncmsg.SendId),
		SendId : syncmsg.SendId,
	}
	pushmsg.PushParams.UserId = []string{"letv_"+ syncmsg.UserId}
	pushmsg.Options.TTL = 60
	b, _ := json.Marshal(pushmsg)
	client := &http.Client{}
	req, err := http.NewRequest("POST", pushurl, bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", fmt.Sprintf("LETV %s pushtest", syncmsg.AppId))
	log.Infof("push msg...")
	resp, err := client.Do(req)
	resp.Body.Close()
	return nil, err
	//return resp, err
}

func addMessage(w http.ResponseWriter, r *http.Request) {
	body, _ := ioutil.ReadAll(r.Body)
	log.Infof("post body (%s)", string(body))
	// decode JSON body
	msg := SyncMsg{}
	if err := json.Unmarshal(body, &msg); err != nil {
		log.Error(err)
		errResponse(w, ERR_BAD_REQUEST, "json decode body failed", 400)
		return
	}

	msgBox <- &msg

	var response Response
	response.ErrNo = 0
	b, _ := json.Marshal(response)
	fmt.Fprintf(w, string(b))
}

func messageHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	var response Response
	switch r.Method {
	case "POST":
		addMessage(w, r)
		return
	default:
	}
	response.ErrNo = ERR_METHOD_NOT_ALLOWED
	response.ErrMsg = "Method not allowed"
	b, _ := json.Marshal(response)
	http.Error(w, string(b), 405)
	return
}

func startHttp(addr string) {
	log.Infof("web server routine start")
	// push API
	http.HandleFunc("/sync/message", messageHandler)
	err := http.ListenAndServe(addr, nil)
	if err != nil {
		log.Criticalf("http listen: ", err)
		os.Exit(1)
	}
	log.Infof("web server routine stop")
}

func main() {
	var (
		configFile    = flag.String("c", "./conf/conf.json", "Config file")
		logConfigFile = flag.String("l", "./conf/log.xml", "Log config file")
	)
	flag.Parse()

	err := conf.LoadConfig(*configFile)
	if err != nil {
		fmt.Printf("LoadConfig (%s) failed: (%s)\n", *configFile, err)
		os.Exit(1)
	}

	err = log.RegisterCustomFormatter("Ms", utils.CreateMsFormatter)
	if err != nil {
		fmt.Printf("Failed to create custom formatter: (%s)\n", err)
		os.Exit(1)
	}

	logger, err := log.LoggerFromConfigAsFile(*logConfigFile)
	if err != nil {
		fmt.Printf("Load log config failed: (%s)\n", err)
		os.Exit(1)
	}

	log.ReplaceLogger(logger)

	wg := &sync.WaitGroup{}
	c := make(chan os.Signal, 1)
	ctrl := make(chan bool, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

	wg.Add(1)
	go func() {
		sig := <-c
		log.Infof("Received signal '%v', exiting\n", sig)
		close(ctrl)
		wg.Done()
	}()

	go startHttp(conf.Config.Sync.Url)
	waitingMsgs := make(map[string]*SyncMsg)
	go func() {
		log.Infof("message handler routine start")
		wg.Add(1)
		for {
			select {
			case _ = <-ctrl:
				log.Infof("message handler routine stop")
				wg.Done()
				return
			//case msg, ok := <-msgBox:
			case msg := <-msgBox:
				key := fmt.Sprintf("%s_%s_%s_%s", msg.AppId, msg.UserId, msg.SendId, msg.RegId)
				if _, ok := waitingMsgs[key]; !ok {
					waitingMsgs[key] = msg
				}
			case <-time.After(conf.Config.Sync.Interval * time.Second):
				for _, msg := range(waitingMsgs) {
					push(msg)
				}
				waitingMsgs = map[string]*SyncMsg{}
			}
		}
	}()
	wg.Wait()
}

