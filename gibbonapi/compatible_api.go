package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	log "github.com/cihub/seelog"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chenyf/gibbon/comet"
	"github.com/chenyf/gibbon/devcenter"

	"github.com/fighterlyt/permutation"
)

const (
	STATUS_OK                = 0
	STATUS_OTHER_ERR         = -1
	STATUS_ROUTER_NOT_LOGGIN = -2
	STATUS_TASK_NOT_EXIST    = -3
	STATUS_TASK_EXIST        = -4
	STATUS_INVALID_PARAM     = -5
	STATUS_ROUTER_OFFLINE    = -6
)

type CommandResponse struct {
	Status int    `json:"status"`
	Error  string `json:"error"`
}

var (
	permutations [][]int
)

func initPermutation() error {
	a := []int{0, 1, 2, 3, 4, 5, 6}

	p, err := permutation.NewPerm(a, nil)
	if err != nil {
		return err
	}

	result := p.NextN(p.Left())
	permutations = result.([][]int)

	log.Infof("Generated %d permutations", len(permutations))

	return nil
}

func sign_calc(path string, query map[string]string) string {
	const key string = "xnRzFxoCDRVRU2mNQ7AoZ5MCxpAR7ntnmlgRGYav"

	uid := query["uid"]
	rid := query["rid"]
	tid := query["tid"]
	src := query["src"]
	tm := query["tm"]
	pmtt := query["pmtt"]

	raw := []string{path, uid, rid, tid, src, tm, pmtt}
	perm_index, err := strconv.Atoi(pmtt)
	if err != nil || perm_index < 0 || perm_index >= len(permutations) {
		return ""
	}

	perm := permutations[perm_index]
	//log.Debugf("perm: %v", perm)
	args := []string{}
	for _, item := range perm {
		args = append(args, raw[item])
	}
	data := strings.Join(args, "")
	//log.Debugf(data)
	mac := hmac.New(sha1.New, []byte(key))
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

func checkAuthzUid(uid string, devid string) bool {
	// TODO: remove this is for test
	if uid == "000000000" {
		return true
	}

	devices, err := devcenter.GetDevices(uid, devcenter.DEV_ROUTER)
	if err != nil {
		log.Errorf("GetDevices failed: %s", err.Error())
		return false
	}

	for _, dev := range devices {
		if devid == dev.Id {
			return true
		}
	}
	return false
}

func postRouterCommand(w http.ResponseWriter, r *http.Request) {
	log.Tracef("postRouterCommand")
	log.Debugf("Request from RemoterAddr: %s", r.RemoteAddr)
	var (
		uid        string
		rid        string
		tid        string
		sign       string
		tm         string
		pmtt       string
		path       string = "/router/command"
		mysign     string
		query      map[string]string
		response   CommandResponse
		client     *comet.Client
		body       []byte
		err        error
		cmdRequest comet.CommandRequest
		bCmd       []byte
		reply      chan *comet.Message
		seq        uint32
	)
	response.Status = STATUS_OK
	if r.Method != "POST" {
		response.Status = STATUS_OTHER_ERR
		response.Error = "must use 'POST' method\n"
		goto resp
	}

	r.ParseForm()
	rid = r.FormValue("rid")
	if rid == "" {
		response.Status = STATUS_INVALID_PARAM
		response.Error = "missing 'rid'"
		goto resp
	}

	uid = r.FormValue("uid")
	if uid == "" {
		response.Status = STATUS_INVALID_PARAM
		response.Error = "missing 'uid'"
		goto resp
	}

	tid = r.FormValue("tid")
	if tid == "" {
		response.Status = STATUS_INVALID_PARAM
		response.Error = "missing 'tid'"
		goto resp
	}

	sign = r.FormValue("sign")
	if sign == "" {
		response.Status = STATUS_INVALID_PARAM
		response.Error = "missing 'sign'"
		goto resp
	}

	tm = r.FormValue("tm")
	if tm == "" {
		response.Status = STATUS_INVALID_PARAM
		response.Error = "missing 'tm'"
		goto resp
	}

	pmtt = r.FormValue("pmtt")
	if pmtt == "" {
		response.Status = STATUS_INVALID_PARAM
		response.Error = "missing 'pmtt'"
		goto resp
	}

	query = map[string]string{
		"uid":  uid,
		"rid":  rid,
		"tid":  tid,
		"src":  "src",
		"tm":   tm,
		"pmtt": pmtt,
	}
	mysign = sign_calc(path, query)
	if mysign != sign {
		//response.Error = "sign valication failed"
		//response.Status = STATUS_INVALID_PARAM
		//goto resp
		log.Warnf("mysign: %s", mysign)
		log.Warnf("insign: %s", sign)
		log.Warnf("sign valication failed")
	}

	if !checkAuthzUid(uid, rid) {
		log.Warnf("auth failed. uid: %s, rid: %s", uid, rid)
		response.Status = STATUS_OTHER_ERR
		response.Error = "authorization failed"
		goto resp
	}

	if r.Body == nil {
		response.Status = STATUS_INVALID_PARAM
		response.Error = "missing POST data"
		goto resp
	}

	if !comet.DevMap.Check(rid) {
		response.Status = STATUS_ROUTER_OFFLINE
		response.Error = fmt.Sprintf("device (%s) offline", rid)
		goto resp
	}
	client = comet.DevMap.Get(rid).(*comet.Client)

	body, err = ioutil.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		response.Status = STATUS_INVALID_PARAM
		response.Error = "invalid POST body"
		goto resp
	}

	cmdRequest = comet.CommandRequest{
		Uid: uid,
		Cmd: string(body),
	}

	bCmd, _ = json.Marshal(cmdRequest)
	reply = make(chan *comet.Message)
	seq = client.SendMessage(comet.MSG_ROUTER_COMMAND, bCmd, reply)
	select {
	case msg := <-reply:
		close(reply)
		w.Write(msg.Data)
	case <-time.After(time.Duration(commandTimeout) * time.Second):
		client.MsgTimeout(seq)
		close(reply)
		response.Status = STATUS_OTHER_ERR
		response.Error = fmt.Sprintf("recv response [%d] timeout on device [%s]", seq, client.DevId)
		goto resp
	}
	return

resp:
	b, _ := json.Marshal(response)
	log.Debugf("postRouterCommand write: %s", string(b))
	w.Write(b)
}

func getRouterList(w http.ResponseWriter, r *http.Request) {
	log.Tracef("getRouterList")

	type RouterInfo struct {
		Rid   string `json:"rid"`
		Rname string `json:"rname"`
	}

	type ResponseRouterList struct {
		Status int          `json:"status"`
		Descr  string       `json:"descr"`
		List   []RouterInfo `json:"list"`
	}

	var (
		uid      string
		tid      string
		sign     string
		tm       string
		pmtt     string
		path     string = "/router/list"
		query    map[string]string
		mysign   string
		response ResponseRouterList
		router   RouterInfo
		devices  []devcenter.Device
		err      error
	)

	response.Status = STATUS_OTHER_ERR
	if r.Method != "GET" {
		response.Descr = "must using 'GET' method\n"
		goto resp
	}
	r.ParseForm()

	uid = r.FormValue("uid")
	if uid == "" {
		response.Status = STATUS_INVALID_PARAM
		response.Descr = "missing 'uid'"
		goto resp
	}

	tid = r.FormValue("tid")
	if tid == "" {
		response.Status = STATUS_INVALID_PARAM
		response.Descr = "missing 'tid'"
		goto resp
	}

	sign = r.FormValue("sign")
	if sign == "" {
		response.Status = STATUS_INVALID_PARAM
		response.Descr = "missing 'sign'"
		goto resp
	}

	tm = r.FormValue("tm")
	if tm == "" {
		response.Status = STATUS_INVALID_PARAM
		response.Descr = "missing 'tm'"
		goto resp
	}

	pmtt = r.FormValue("pmtt")
	if pmtt == "" {
		response.Status = STATUS_INVALID_PARAM
		response.Descr = "missing 'pmtt'"
		goto resp
	}

	query = map[string]string{
		"uid":  uid,
		"rid":  "rid",
		"tid":  tid,
		"src":  "src",
		"tm":   tm,
		"pmtt": pmtt,
	}
	mysign = sign_calc(path, query)
	if mysign != sign {
		//response.Error = "sign valication failed"
		//response.Status = STATUS_INVALID_PARAM
		//goto resp
		log.Warnf("mysign: %s", mysign)
		log.Warnf("insign: %s", sign)
		log.Warnf("sign valication failed")
		//TODO:
		//goto resp
	}

	devices, err = devcenter.GetDevices(uid, devcenter.DEV_ROUTER)
	if err != nil {
		log.Errorf("GetDevices failed: %s", err.Error())
		response.Descr = err.Error()
		goto resp
	}

	for _, dev := range devices {
		router = RouterInfo{
			Rid:   dev.Id,
			Rname: dev.Title,
		}
		response.List = append(response.List, router)
	}

	response.Status = 0
	response.Descr = "OK"

resp:
	b, _ := json.Marshal(response)
	log.Debugf("getRoutelist write: %s", string(b))
	w.Write(b)
}
