package main

import (
	"flag"
	"os"
	"log"
	"sync"
	"strings"
	"io/ioutil"
	"os/signal"
	"syscall"
	"net/http"
	"fmt"
	"time"
	"encoding/json"
	"crypto/hmac"
	"crypto/sha1"
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

func getStatus(w http.ResponseWriter, r *http.Request) {
	size := comet.DevMap.Size()
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
	rid := r.FormValue("rid")
	if rid == "" {
		response.Error = "missing 'rid'"
		b, _ := json.Marshal(response)
		fmt.Fprintf(w, string(b))
		return
	}

	uid := r.FormValue("uid")
	if uid == "" {
		response.Error = "missing 'uid'"
		b, _ := json.Marshal(response)
		fmt.Fprintf(w, string(b))
		return
	}

	if !checkAuthz(uid, rid) {
		response.Error = "authorization failed"
		b, _ := json.Marshal(response)
		fmt.Fprintf(w, string(b))
		return
	}

	/*
	uid := r.FormValue("uid")
	if uid == "" {	fmt.Fprintf(w, "missing 'uid'\n"); return; }
	tid := r.FormValue("tid")
	if tid == "" {	fmt.Fprintf(w, "missing 'tid'\n"); return; }
	sign := r.FormValue("sign")
	if sign == "" {	fmt.Fprintf(w, "missing 'sign'\n"); return; }
	tm := r.FormValue("tm")
	if tm == "" {	fmt.Fprintf(w, "missing 'tm'\n"); return; }
	pmtt := r.FormValue("pmtt")
	if pmtt == "" {	fmt.Fprintf(w, "missing 'pmtt'\n"); return; }
	query := map[string]string {
		"uid" : uid,
		"rid" : rid,
		"tid" : tid,
		"src" : "letv",
		"tm" :  tm,
		"pmtt" : pmtt,
	}
	path := "/router/command"
	mysign := sign(path, query)
	if mysign != sign {
		response.Error = "sign valication failed"
		b, _ := json.Marshal(response)
		fmt.Fprintf(w, string(b))
		return
	}
	*/

	if r.Body == nil {
		response.Error = "missing POST data"
		b, _ := json.Marshal(response)
		fmt.Fprintf(w, string(b))
		return
	}

	if !comet.DevMap.Check(rid) {
		response.Error = fmt.Sprintf("device (%s) offline", rid)
		b, _ := json.Marshal(response)
		fmt.Fprintf(w, string(b))
		return
	}
	client := comet.DevMap.Get(rid).(*comet.Client)

	body, err := ioutil.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		response.Error = "invalid POST body"
		b, _ := json.Marshal(response)
		fmt.Fprintf(w, string(b))
		return
	}

	cmdRequest := CommandRequest{
		Uid: uid,
		Cmd: string(body),
	}

	bCmd, _ := json.Marshal(cmdRequest)
	reply := make(chan *comet.Message)
	client.SendMessage(comet.MSG_REQUEST, bCmd, reply)
	select {
	case msg := <-reply:
		fmt.Fprintf(w, string(msg.Data))
	case <- time.After(10 * time.Second):
		response.Error = "recv response timeout"
		b, _ := json.Marshal(response)
		fmt.Fprintf(w, string(b))
	}
}

func getCommand(w http.ResponseWriter, r *http.Request) {

	r.ParseForm()
	devid := r.FormValue("devid")
	if devid == "" {
		fmt.Fprintf(w, "missing devid\n")
		return
	}
	if !comet.DevMap.Check(devid) {
		fmt.Fprintf(w, "(%s) not register\n", devid)
		return
	}
	cmd := r.FormValue("cmd")
	client := comet.DevMap.Get(devid).(*comet.Client)
	reply := make(chan *comet.Message)
	client.SendMessage(comet.MSG_REQUEST, []byte(cmd), reply)
	select {
	case msg := <-reply:
		fmt.Fprintf(w, "recv reply  (%s)\n", string(msg.Data))
	case <- time.After(10 * time.Second):
		fmt.Fprintf(w, "recv timeout\n")
	}
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

	listener, err := cometServer.Init("0.0.0.0:10000")
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
		waitGroup.Done()
		log.Printf("leave 2")
	}()

	go func() {
		cometServer.Run(listener)
	}()
	waitGroup.Add(1)
	go func() {
		http.HandleFunc("/router/command", postRouterCommand)
		http.HandleFunc("/command", getCommand)
		http.HandleFunc("/status", getStatus)
		err := http.ListenAndServe("0.0.0.0:9999", nil)
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

