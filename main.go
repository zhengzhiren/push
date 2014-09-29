package main

import (
	"flag"
	"os"
	"log"
	"sync"
	"strings"
	"strconv"
	"io/ioutil"
	"os/signal"
	"syscall"
	"net/http"
	"fmt"
	//"time"
	"encoding/json"
	"crypto/hmac"
	"crypto/sha1"
	"github.com/chenyf/push/mq"
	"github.com/chenyf/push/comet"
)

type CommandRequest struct {
	Uid		string	`json:"uid"`
	Cmd		string	`json:"cmd"`
}

type CommandResponse struct {
	Status	int		`json:"status"`
	Error	string	`json:"error"`
}

var (
	uri          = flag.String("uri", "amqp://guest:guest@10.154.156.121:5672/", "AMQP URI")
	exchange     = flag.String("exchange", "test-exchange", "Durable, non-auto-deleted AMQP exchange name")
	exchangeType = flag.String("exchange-type", "direct", "Exchange type - direct|fanout|topic|x-custom")
	queue        = flag.String("queue", "test-queue", "Ephemeral AMQP queue name")
	bindingKey   = flag.String("key", "test-key", "AMQP binding key")
	consumerTag  = flag.String("consumer-tag", "simple-consumer", "AMQP consumer tag (should not be blank)")
	qos          = flag.Int("qos", 1, "message qos")
)

func getStatus(w http.ResponseWriter, r *http.Request) {
	size := comet.DevicesMap.Size()
	fmt.Fprintf(w, "total register device: %d\n", size)
}

func checkAuthz(uid string, devid string) bool {
	return false
}

func sign(path string, query map[string]string) []byte{
	uid := query["uid"]
	rid := query["rid"]
	tid := query["tid"]
	src := query["src"]
	tm := query["tm"]
	pmtt := query["pmtt"]

	raw := []string{path, uid, rid, tid, src, tm, pmtt}
	args := []string{}
	x := []int{6, 5, 4, 3, 2, 1, 0}
	for _, item := range(x) {
		args = append(args, raw[item])
	}
	data := strings.Join(args, "")
	key := "xnRzFxoCDRVRU2mNQ7AoZ5MCxpAR7ntnmlgRGYav"
	mac := hmac.New(sha1.New, []byte(key))
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

type InputMsg struct {
	AppId		string	`json:"app_id"`
	PushType	int		`json:"push_type"`
	RegId		string	`json:"reg_id"`
	MsgType		int		`json:"msg_type"`
	TTL			uint64	`json:"ttl"`
	MsgId		int64	`json:"msg_id"`
	Payload		string	`json:"payload"`
}
func postRouterCommand(w http.ResponseWriter, r *http.Request) {
	var response CommandResponse
	response.Status = 1
	if r.Method != "POST" {
		response.Error = "must using 'POST' method\n"
		b, _ := json.Marshal(response)
		fmt.Fprintf(w, string(b))
		return
	}
	r.ParseForm()
	if r.Body == nil {
		response.Error = "missing POST data"
		b, _ := json.Marshal(response)
		fmt.Fprintf(w, string(b))
		return
	}
	body, err := ioutil.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		response.Error = "invalid POST body"
		b, _ := json.Marshal(response)
		fmt.Fprintf(w, string(b))
		return
	}

	var input InputMsg
	if err := json.Unmarshal(body, &input); err != nil {
		response.Error = fmt.Sprintf("json decode failed, %s", err)
		b, _ := json.Marshal(response)
		fmt.Fprintf(w, string(b))
		return
	}

	msg := comet.PushMessage{
		MsgId : input.MsgId,
		AppId : input.AppId,
		Type : input.MsgType,
		Content : input.Payload,
	}
	b, _ := json.Marshal(msg)
	regid := ""
	if input.PushType == 1 {
		regid = input.RegId
	}
	comet.PushOutMessage(input.AppId, input.PushType, regid, b)
}

func getCommand(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	appid := r.FormValue("appid")
	if appid == "" {
		fmt.Fprintf(w, "missing appid\n")
		return
	}
	push_type := 0
	ptype := r.FormValue("ptype")
	if ptype != "" {
		push_type, _ = strconv.Atoi(ptype)
	}
	msgid := r.FormValue("msgid")
	if msgid == "" {
		fmt.Fprintf(w, "missing msgid\n")
		return
	}
	mid, err := strconv.Atoi(msgid)
	if err != nil {
		fmt.Fprintf(w, "invalid msgid\n")
		return
	}
	cmd := r.FormValue("cmd")
	msg := comet.PushMessage{
		MsgId : int64(mid),
		AppId : appid,
		Type : 0,
		Content : cmd,
	}
	b, _ := json.Marshal(msg)
	comet.PushOutMessage(appid, push_type, "", b)
}

func main() {

	var (
		//flRoot               = flag.String("g", "/tmp/echoserver", "Path to use as the root of the docker runtime")
	)

	flag.Parse()

	/*job := eng.Job("init")
	if err := job.Run(); err != nil {
		log.Fatal(err)
	}
	*/

	waitGroup := &sync.WaitGroup{}
	cometServer := comet.NewServer()
	mqConsumer, err := mq.NewConsumer(*uri, *exchange, *exchangeType, *queue, *bindingKey, *consumerTag, *qos)
	if err != nil {
			log.Fatal(err)
	}

	listener, err := cometServer.Init("0.0.0.0:20000")
	if err != nil {
		log.Fatal(err)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		sig := <-c
		log.Printf("Received signal '%v', exiting\n", sig)
		///utils.RemovePidFile(srv.	runtime.config.Pidfile)
		cometServer.Stop()
		log.Printf("leave 1")
		mqConsumer.Shutdown()
		waitGroup.Done()
		log.Printf("leave 2")
	}()

	go func() {
		cometServer.Run(listener)
	}()

	go func() {
		log.Printf("mq running")
		mqConsumer.Consume()
	}()
	waitGroup.Add(1)
	go func() {
		http.HandleFunc("/router/command", postRouterCommand)
		http.HandleFunc("/command", getCommand)
		http.HandleFunc("/status", getStatus)
		err := http.ListenAndServe("0.0.0.0:19999", nil)
		if err != nil {
			log.Fatal("http listen: ", err)
		}
	}()
	/*
	job = eng.Job("restapi")
	job.SetenvBool("Logging", true)
	if err := job.Run(); err != nil {
		log.Fatal(err)
	}
	*/
	waitGroup.Wait()
}

