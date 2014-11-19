package main

import (
	"encoding/json"
	"fmt"
	"github.com/chenyf/push/auth"
	"github.com/chenyf/push/storage"
	"github.com/chenyf/push/utils"
	"github.com/chenyf/push/zk"
	log "github.com/cihub/seelog"
	uuid "github.com/codeskyblue/go-uuid"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	TimeToLive  = "ttl"
	MsgID       = "msgid"
	PappID      = "pappid" //appid prefix
	MaxMsgCount = 9223372036854775807
)

const (
	ERR_INTERNAL           = 1000
	ERR_METHOD_NOT_ALLOWED = 1001
	ERR_BAD_REQUEST        = 1002
	ERR_INVALID_PARAMS     = 1003
	ERR_AUTHENTICATE       = 1004
	ERR_AUTHORIZE          = 1005
	ERR_EXIST              = 2001
	ERR_NOT_EXIST          = 2002
)

type Response struct {
	ErrNo  int         `json:"errno"`
	ErrMsg string      `json:"errmsg,omitempty"`
	Data   interface{} `json:"data,omitempty"`
}

var msgBox = make(chan storage.RawMessage, 10)

func checkMessage(m *storage.RawMessage) (bool, string) {
	if m.AppSec == "" && m.Token == "" {
		return false, "must specify 'appsec' or 'token'"
	}
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
	if m.PushType == 2 && (m.PushParams.RegId == nil || len(m.PushParams.RegId) == 0) {
		return false, "empty 'regid' when 'push_type'==2"
	}
	if m.PushType == 3 && (m.PushParams.UserId == nil || len(m.PushParams.UserId) == 0) {
		return false, "empty 'userid' when 'push_type'==3"
	}
	if m.PushType == 4 && (m.PushParams.DevId == nil || len(m.PushParams.DevId) == 0) {
		return false, "empty 'devid' when 'push_type'==4"
	}
	if m.PushType == 5 && m.PushParams.Topic == "" {
		return false, "empty 'topic' when 'push_type'==5"
	}
	if m.Options.TTL > 3*86400 {
		return false, "invalid 'options.ttl'"
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

func setPackage(uid string, appId string, appKey string, appSec string, pkg string) error {
	rawapp := storage.RawApp{
		Pkg:    pkg,
		UserId: uid,
		AppKey: appKey,
		AppSec: appSec,
	}
	b, _ := json.Marshal(rawapp)
	if _, err := storage.Instance.HashSet("db_apps", appId, b); err != nil {
		log.Infof("failed to set 'db_apps': %s", err)
		return err
	}
	if _, err := storage.Instance.HashSet("db_packages", pkg, []byte(appId)); err != nil {
		log.Infof("failed to set 'db_packages': %s", err)
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
	}
	response.ErrNo = 0
	response.Data = map[string][]string{"servers": node}
	b, _ := json.Marshal(response)
	fmt.Fprintf(w, string(b))
}

func testHandler(w http.ResponseWriter, r *http.Request) {
	var response Response
	if r.Method != "POST" {
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
	case "GET":
		getApp(w, r)
		return
	default:
	}
	response.ErrNo = ERR_METHOD_NOT_ALLOWED
	response.ErrMsg = "Method not allowed"
	b, _ := json.Marshal(response)
	http.Error(w, string(b), 405)
	return
}

func getApp(w http.ResponseWriter, r *http.Request) {
	var response Response
	pkg := r.FormValue("pkg")
	if pkg == "" {
		response.ErrNo = ERR_INVALID_PARAMS
		response.ErrMsg = "missing 'pkg'"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 400)
		return
	}
	appid, err := storage.Instance.HashGet("db_packages", pkg)
	if err != nil {
		response.ErrNo = ERR_INTERNAL
		response.ErrMsg = "storage I/O failed"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 500)
		return
	}
	if appid == nil {
		response.ErrNo = ERR_NOT_EXIST
		response.ErrMsg = "package not exist"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 400)
		return
	}
	response.ErrNo = 0
	response.Data = map[string]string{"appid": string(appid)}
	b, _ := json.Marshal(response)
	fmt.Fprintf(w, string(b))
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
		response.ErrNo = ERR_INVALID_PARAMS
		response.ErrMsg = "missing 'pkg'"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 400)
		return
	}
	token, ok := data["token"]
	if !ok {
		response.ErrNo = ERR_INVALID_PARAMS
		response.ErrMsg = "missing 'token'"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 400)
		return
	}
	ok, uid := auth.Instance.Auth(token)
	if !ok {
		response.ErrNo = ERR_AUTHENTICATE
		response.ErrMsg = "authenticate failed"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 401)
		return
	}
	tprefix := getPappID()
	if tprefix == 0 {
		response.ErrNo = ERR_INTERNAL
		response.ErrMsg = "no avaiabled appid"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 500)
		return
	}
	n, err := storage.Instance.HashExists("db_packages", pkg)
	if err != nil {
		response.ErrNo = ERR_INTERNAL
		response.ErrMsg = "storage I/O failed"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 500)
		return
	}
	if n > 0 {
		response.ErrNo = ERR_EXIST
		response.ErrMsg = "package exist"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 400)
		return
	}

	prefix := strconv.FormatInt(tprefix, 10)
	tappid := strings.Replace(uuid.New(), "-", "", -1)
	appId := "appid_" + tappid[0:(len(tappid)-len(prefix))] + prefix
	appKey := "appkey_" + utils.RandomAlphabetic(20)
	appSec := "appsec_" + utils.RandomAlphabetic(20)
	if err := setPackage(uid, appId, appKey, appSec, pkg); err != nil {
		response.ErrNo = ERR_INTERNAL
		response.ErrMsg = "storage I/O failed"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 500)
		return
	}
	response.ErrNo = 0
	response.Data = map[string]string{
		"appid":  appId,
		"appkey": appKey,
		"appsec": appSec,
	}
	b, _ := json.Marshal(response)
	fmt.Fprintf(w, string(b))
}

func delApp(w http.ResponseWriter, r *http.Request) {
	var response Response
	var data map[string]string
	var b []byte
	var err error
	var ok bool
	if err = json.NewDecoder(r.Body).Decode(&data); err != nil {
		response.ErrNo = ERR_BAD_REQUEST
		response.ErrMsg = "Bad request"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 400)
		return
	}
	pkg, ok := data["pkg"]
	if !ok {
		response.ErrNo = ERR_INVALID_PARAMS
		response.ErrMsg = "missing 'pkg'"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 400)
		return
	}
	token, ok1 := data["token"]
	appsec, ok2 := data["appsec"]
	if !ok1 && !ok2 {
		response.ErrNo = ERR_INVALID_PARAMS
		response.ErrMsg = "must specify 'appsec' or 'token'"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 400)
		return
	}
	var uid string
	if appsec == "" {
		ok, uid = auth.Instance.Auth(token)
		if !ok {
			response.ErrNo = ERR_AUTHENTICATE
			response.ErrMsg = "authenticate failed"
			b, _ := json.Marshal(response)
			http.Error(w, string(b), 401)
			return
		}
	}
	b, err = storage.Instance.HashGet("db_packages", pkg)
	if err != nil {
		response.ErrNo = ERR_INTERNAL
		response.ErrMsg = "storage I/O failed"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 500)
		return
	}
	if b == nil {
		response.ErrNo = ERR_NOT_EXIST
		response.ErrMsg = "package not exist"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 400)
		return
	}
	appid := string(b)
	b, err = storage.Instance.HashGet("db_apps", appid)
	if err != nil {
		response.ErrNo = ERR_INTERNAL
		response.ErrMsg = "storage I/O failed"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 500)
		return
	}
	var rawapp storage.RawApp
	json.Unmarshal(b, &rawapp)
	authz_ok := false
	if appsec != "" && appsec == rawapp.AppSec {
		authz_ok = true
	} else if uid != "" && uid == rawapp.UserId {
		authz_ok = true
	}
	if !authz_ok {
		response.ErrNo = ERR_AUTHORIZE
		response.ErrMsg = "authorize failed"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 400)
		return
	}

	if _, err = storage.Instance.HashDel("db_apps", appid); err != nil {
		response.ErrNo = ERR_INTERNAL
		response.ErrMsg = "storage I/O failed"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 500)
		return
	}
	if _, err = storage.Instance.HashDel("db_packages", pkg); err != nil {
		response.ErrNo = ERR_INTERNAL
		response.ErrMsg = "storage I/O failed"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 500)
		return
	}
	response.ErrNo = 0
	b, _ = json.Marshal(response)
	fmt.Fprintf(w, string(b))
}

func addMessage(w http.ResponseWriter, r *http.Request) {
	var response Response
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
		response.ErrNo = ERR_INVALID_PARAMS
		response.ErrMsg = desc
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 400)
		return
	}
	appsec := msg.AppSec
	uid := ""
	if appsec == "" { //use 'token'
		ok, uid = auth.Instance.Auth(msg.Token)
		if !ok {
			response.ErrNo = ERR_AUTHENTICATE
			response.ErrMsg = "authenticate failed"
			b, _ := json.Marshal(response)
			http.Error(w, string(b), 401)
			return
		}
	}
	b, err := storage.Instance.HashGet("db_apps", msg.AppId)
	if err != nil {
		response.ErrNo = ERR_INTERNAL
		response.ErrMsg = "storage I/O failed"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 500)
		return
	}
	if b == nil {
		response.ErrNo = ERR_NOT_EXIST
		response.ErrMsg = "app not exist"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 400)
		return
	}
	var rawapp storage.RawApp
	if err := json.Unmarshal(b, &rawapp); err != nil {
		log.Warnf("unmarshal failed, (%s)", err)
	}
	authz_ok := false
	if appsec != "" && appsec == rawapp.AppSec {
		authz_ok = true
	} else if uid != "" && uid == rawapp.UserId {
		authz_ok = true
	}
	if !authz_ok {
		response.ErrNo = ERR_AUTHORIZE
		response.ErrMsg = "authorize failed"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 400)
		return
	}
	msgid := getMsgID()
	if msgid == 0 {
		response.ErrNo = ERR_INTERNAL
		response.ErrMsg = "no avaiabled msgid"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 500)
		return
	}
	msg.MsgId = msgid
	response.ErrNo = 0
	response.Data = map[string]int64{"msgid": msgid}
	msg.CTime = time.Now().Unix()
	msgBox <- msg
	b, _ = json.Marshal(response)
	fmt.Fprintf(w, string(b))
}

func getMessage(w http.ResponseWriter, r *http.Request) {
	var response Response
	msgid := r.FormValue("msgid")
	if msgid == "" {
		response.ErrNo = ERR_INVALID_PARAMS
		response.ErrMsg = "missing 'msgid'"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 400)
		return
	}
	b, err := storage.Instance.HashGet("db_msg_stat", msgid)
	if err != nil {
		response.ErrNo = ERR_INTERNAL
		response.ErrMsg = "storage I/O failed"
		b, _ := json.Marshal(response)
		http.Error(w, string(b), 500)
		return
	}
	response.ErrNo = 0
	response.Data = map[string]string{"send": string(b)}
	b, err = json.Marshal(response)
	if err != nil {
		log.Warnf("error (%s)", err)
	}
	fmt.Fprintf(w, string(b))
}

func messageHandler(w http.ResponseWriter, r *http.Request) {
	var response Response
	switch r.Method {
	case "POST":
		addMessage(w, r)
		return
	case "GET":
		getMessage(w, r)
		return
	default:
	}
	response.ErrNo = ERR_METHOD_NOT_ALLOWED
	response.ErrMsg = "Method not allowed"
	b, _ := json.Marshal(response)
	http.Error(w, string(b), 405)
	return
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
