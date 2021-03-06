package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ant0ine/go-json-rest/rest"
	log "github.com/cihub/seelog"
	uuid "github.com/codeskyblue/go-uuid"

	"github.com/chenyf/push/auth"
	"github.com/chenyf/push/storage"
	"github.com/chenyf/push/utils"
	"github.com/chenyf/push/zk"
)

const (
	TimeToLive  = "ttl"
	MsgID       = "msgid"
	PappID      = "pappid" //appid prefix
	MaxMsgCount = 9223372036854775807
	ADMIN_SIGN  = "pushtest"
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

var msgBox = make(chan storage.RawMessage, 10)

func checkMessage(m *storage.RawMessage) (bool, string) {
	if m.AppId == "" {
		return false, "missing 'appid'"
	}
	if m.MsgType < 1 || m.MsgType > 2 {
		return false, "invalid 'msg_type'"
	}
	if m.MsgType == 2 {
		if m.Content == "" {
			return false, "missing 'content' or empty 'content' when 'msg_type'==2"
		}
	} else {
		if m.Notification.Title == "" {
			return false, "missing 'notification.title' or empty when 'msg_type'==1"
		}
	}
	if m.PushType < 1 || m.PushType > 7 {
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
	if m.PushType == 5 && len(m.PushParams.Topic) == 0 {
		return false, "empty 'topic' when 'push_type'==5"
	}
	if m.PushType == 7 && len(m.PushParams.Group) == 0 {
		return false, "empty 'group' when 'push_type'==7"
	}
	if m.PushParams.TopicOp != "" &&
		m.PushParams.TopicOp != "or" &&
		m.PushParams.TopicOp != "and" &&
		m.PushParams.TopicOp != "except" {
		return false, "invalid 'topic_op'"
	}
	if m.Options.TTL > 3*86400 {
		return false, "invalid 'options.ttl'"
	}
	return true, ""
}

func errResponse(w http.ResponseWriter, errno int, errmsg string, httpcode int) {
	var response Response
	response.ErrNo = errno
	response.ErrMsg = errmsg
	b, _ := json.Marshal(response)
	http.Error(w, string(b), httpcode)
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

func serverHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		errResponse(w, ERR_METHOD_NOT_ALLOWED, "method not allowed", 405)
		return
	}
	node := zk.GetComet()
	if node == nil {
		node = []string{}
	}
	var response Response
	response.ErrNo = 0
	response.Data = map[string][]string{"servers": node}
	b, _ := json.Marshal(response)
	fmt.Fprintf(w, string(b))
}

func testHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		errResponse(w, ERR_METHOD_NOT_ALLOWED, "method not allowed", 405)
		return
	}
	var response Response
	response.ErrNo = 0
	b, _ := json.Marshal(response)
	fmt.Fprintf(w, string(b))
}

func getApp(w http.ResponseWriter, r *http.Request) {
	pkg := r.FormValue("pkg")
	if pkg == "" {
		errResponse(w, ERR_INVALID_PARAMS, "missing 'pkg'", 400)
		return
	}
	appid, err := storage.Instance.HashGet("db_packages", pkg)
	if err != nil {
		errResponse(w, ERR_INTERNAL, "storage I/O failed", 500)
		return
	}
	if appid == nil {
		errResponse(w, ERR_NOT_EXIST, "app not exist", 400)
		return
	}
	var response Response
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
		errResponse(w, ERR_INVALID_PARAMS, "missing 'pkg'", 400)
		return
	}
	token, ok := data["token"]
	if !ok {
		errResponse(w, ERR_INVALID_PARAMS, "missing 'token'", 400)
		return
	}
	ok, uid := auth.Instance.Auth(token)
	if !ok {
		errResponse(w, ERR_AUTHENTICATE, "authenticate failed", 401)
		return
	}
	tprefix := getPappID()
	if tprefix == 0 {
		errResponse(w, ERR_INTERNAL, "no avaiable appid", 500)
		return
	}
	n, err := storage.Instance.HashExists("db_packages", pkg)
	if err != nil {
		errResponse(w, ERR_INTERNAL, "storage I/O failed", 500)
		return
	}
	if n > 0 {
		errResponse(w, ERR_EXIST, "package exist", 400)
		return
	}

	prefix := strconv.FormatInt(tprefix, 10)
	tappid := strings.Replace(uuid.New(), "-", "", -1)
	appId := "appid_" + tappid[0:(len(tappid)-len(prefix))] + prefix
	appKey := "appkey_" + utils.RandomAlphabetic(20)
	appSec := "appsec_" + utils.RandomAlphabetic(20)
	rawapp := storage.RawApp{
		Pkg:    pkg,
		UserId: uid,
		AppKey: appKey,
		AppSec: appSec,
	}
	b, _ := json.Marshal(rawapp)
	if _, err := storage.Instance.HashSet("db_apps", appId, b); err != nil {
		errResponse(w, ERR_INTERNAL, "set 'db_apps' failed", 500)
		return
	}
	if _, err := storage.Instance.HashSet("db_packages", pkg, []byte(appId)); err != nil {
		errResponse(w, ERR_INTERNAL, "set 'db_packages' failed", 500)
		return
	}
	response.ErrNo = 0
	response.Data = map[string]string{
		"appid":  appId,
		"appkey": appKey,
		"appsec": appSec,
	}
	b, _ = json.Marshal(response)
	fmt.Fprintf(w, string(b))
}

func addApp2(w http.ResponseWriter, r *http.Request) {
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
	name, ok2 := data["name"]
	mobile, ok3 := data["mobile"]
	email, ok4 := data["email"]
	desc, ok5 := data["desc"]
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 {
		errResponse(w, ERR_INVALID_PARAMS, "missing parameters", 400)
		return
	}
	tprefix := getPappID()
	if tprefix == 0 {
		errResponse(w, ERR_INTERNAL, "no avaiable appid", 500)
		return
	}
	n, err := storage.Instance.HashExists("db_packages", pkg)
	if err != nil {
		errResponse(w, ERR_INTERNAL, "storage I/O failed", 500)
		return
	}
	if n > 0 {
		errResponse(w, ERR_EXIST, "package exist", 400)
		return
	}

	prefix := strconv.FormatInt(tprefix, 10)
	tappid := strings.Replace(uuid.New(), "-", "", -1)
	appId := "id_" + tappid[0:(len(tappid)-len(prefix))] + prefix
	appKey := "ak_" + utils.RandomAlphabetic(20)
	appSec := "sk_" + utils.RandomAlphabetic(20)
	rawapp := storage.RawApp{
		Pkg:    pkg,
		Name:   name,
		Mobile: mobile,
		Email:  email,
		Desc:   desc,
		AppKey: appKey,
		AppSec: appSec,
	}
	b, _ := json.Marshal(rawapp)
	if _, err := storage.Instance.HashSet("db_apps", appId, b); err != nil {
		errResponse(w, ERR_INTERNAL, "set 'db_apps' failed", 500)
		return
	}
	if _, err := storage.Instance.HashSet("db_packages", pkg, []byte(appId)); err != nil {
		errResponse(w, ERR_INTERNAL, "set 'db_packages' failed", 500)
		return
	}
	response.ErrNo = 0
	response.Data = map[string]string{
		"appid":  appId,
		"appkey": appKey,
		"appsec": appSec,
	}
	b, _ = json.Marshal(response)
	fmt.Fprintf(w, string(b))
}

func delApp(w http.ResponseWriter, r *http.Request) {
	authstr := r.Header.Get("Authorization")
	date := r.Header.Get("Date")
	auth := strings.Fields(authstr)
	if len(auth) < 3 {
		errResponse(w, ERR_AUTHORIZE, "invalid 'Authorization' header", 400)
		return
	}
	appid := auth[1]
	sign := auth[2]
	b, err := storage.Instance.HashGet("db_apps", appid)
	if err != nil {
		errResponse(w, ERR_INTERNAL, "storage I/O failed", 500)
		return
	}
	if b == nil {
		errResponse(w, ERR_NOT_EXIST, "app not exist", 400)
		return
	}

	var rawapp storage.RawApp
	json.Unmarshal(b, &rawapp)
	body, _ := ioutil.ReadAll(r.Body)
	if sign != ADMIN_SIGN {
		if utils.Sign(rawapp.AppSec, r.Method, r.URL.Path, body, date, r.Form) != sign {
			errResponse(w, ERR_SIGN, "check sign failed", 400)
			return
		}
	}
	if _, err = storage.Instance.HashDel("db_apps", appid); err != nil {
		errResponse(w, ERR_INTERNAL, "del 'db_apps' failed", 500)
		return
	}
	if _, err = storage.Instance.HashDel("db_packages", rawapp.Pkg); err != nil {
		errResponse(w, ERR_INTERNAL, "del 'db_packages' failed", 500)
		return
	}
	var response Response
	response.ErrNo = 0
	b, _ = json.Marshal(response)
	fmt.Fprintf(w, string(b))
}

func appHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
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

func app2Handler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	var response Response
	switch r.Method {
	case "POST":
		addApp2(w, r)
		return
	default:
	}
	response.ErrNo = ERR_METHOD_NOT_ALLOWED
	response.ErrMsg = "Method not allowed"
	b, _ := json.Marshal(response)
	http.Error(w, string(b), 405)
	return
}

func addMessage(w http.ResponseWriter, r *http.Request) {
	authstr := r.Header.Get("Authorization")
	date := r.Header.Get("Date")
	auth := strings.Fields(authstr)
	if len(auth) < 3 {
		errResponse(w, ERR_AUTHORIZE, "invalid 'Authorization' header", 400)
		return
	}
	appid := auth[1]
	sign := auth[2]
	// load app info
	b, err := storage.Instance.HashGet("db_apps", appid)
	if err != nil {
		errResponse(w, ERR_INTERNAL, "get 'db_apps' failed", 500)
		return
	}
	if b == nil {
		errResponse(w, ERR_NOT_EXIST, "app not exist", 400)
		return
	}
	var rawapp storage.RawApp
	json.Unmarshal(b, &rawapp)

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Errorf("read request body failed: (%s) (%s)", r.Body, err)
		errResponse(w, ERR_BAD_REQUEST, "failed read request body", 400)
		return
	}
	// check sign
	if sign != ADMIN_SIGN {
		if utils.Sign(rawapp.AppSec, r.Method, r.URL.Path, body, date, r.Form) != sign {
			errResponse(w, ERR_SIGN, "check sign failed", 400)
			return
		}
	}

	// decode JSON body
	msg := storage.RawMessage{}
	if err := json.Unmarshal(body, &msg); err != nil {
		log.Errorf("JSON decode failed: (%s) (%s)", body, err)
		errResponse(w, ERR_BAD_REQUEST, "json decode body failed", 400)
		return
	}
	msg.AppId = appid
	// check message format
	ok, desc := checkMessage(&msg)
	if !ok {
		errResponse(w, ERR_INVALID_PARAMS, desc, 400)
		return
	}
	msgid := getMsgID()
	if msgid == 0 {
		errResponse(w, ERR_INTERNAL, "no avaiable msgid", 500)
		return
	}
	msg.MsgId = msgid
	if msg.Options.TTL < 0 { // send immediatly
		msg.Options.TTL = 0
	} else if msg.Options.TTL == 0 {
		msg.Options.TTL = 86400 // default
	}

	var response Response
	response.ErrNo = 0
	response.Data = map[string]int64{"msgid": msgid}
	msg.CTime = time.Now().Unix()
	msgBox <- msg
	b, _ = json.Marshal(response)
	fmt.Fprintf(w, string(b))
	storage.Instance.AppStatsPushApi(appid)
}

/*
func addMessage(w http.ResponseWriter, r *http.Request) {
	var response Response
	msg := storage.RawMessage{}
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		errResponse(w, ERR_BAD_REQUEST, "json decode body failed", 400)
		return
	}
	var ok bool
	ok, desc := checkMessage(&msg)
	if !ok {
		errResponse(w, ERR_INVALID_PARAMS, desc, 400)
		return
	}
	appsec := msg.AppSec
	b, err := storage.Instance.HashGet("db_apps", msg.AppId)
	if err != nil {
		errResponse(w, ERR_INTERNAL, "get 'db_apps' failed", 500)
		return
	}
	if b == nil {
		errResponse(w, ERR_NOT_EXIST, "app not exist", 400)
		return
	}
	var rawapp storage.RawApp
	if err := json.Unmarshal(b, &rawapp); err != nil {
		log.Warnf("unmarshal failed, (%s)", err)
	}
	authz_ok := false
	if appsec != "" && appsec == rawapp.AppSec {
		authz_ok = true
	}
	if !authz_ok {
		errResponse(w, ERR_AUTHORIZE, "authorize failed", 400)
		return
	}
	msgid := getMsgID()
	if msgid == 0 {
		errResponse(w, ERR_INTERNAL, "no avaiable msgid", 500)
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
*/

func getMessage(w http.ResponseWriter, r *http.Request) {
	type StatResponse struct {
		MsgId      int64 `json:"msg_id"`
		CreateTime int64 `json:"create_time"`
		Send       int   `json:"send"`
		Received   int   `json:"received"`
		Click      int   `json:"click"`
	}
	appid := r.FormValue("appid")
	msgid := r.FormValue("msgid")
	if appid == "" || msgid == "" {
		errResponse(w, ERR_INVALID_PARAMS, "missing 'appid' or 'msgid'", 400)
		return
	}
	b, err := storage.Instance.HashGet(fmt.Sprintf("db_msg_%s", appid), msgid)
	if err != nil {
		errResponse(w, ERR_INTERNAL, "storage I/O failed", 500)
		return
	}
	if b == nil {
		errResponse(w, ERR_INTERNAL, "no msgid in this appid", 500)
		return
	}

	var rawmsg storage.RawMessage
	err = json.Unmarshal(b, &rawmsg)

	msgId, _ := strconv.Atoi(msgid)
	respData := StatResponse{
		CreateTime: rawmsg.CTime,
		MsgId:      int64(msgId),
	}

	respData.Send, respData.Received, respData.Click, err = storage.Instance.GetMsgStats(int64(msgId))
	if err != nil {
		errResponse(w, ERR_INTERNAL, "storage I/O failed", 500)
		return
	}

	var response Response
	response.ErrNo = 0
	response.Data = respData
	b, err = json.Marshal(response)
	if err != nil {
		log.Warnf("error (%s)", err)
	}
	fmt.Fprintf(w, string(b))
}

func messageHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
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

func getAppStats(w rest.ResponseWriter, r *rest.Request) {
	r.ParseForm()
	appId := r.FormValue("appid")
	startDate := r.FormValue("start_date")
	endDate := r.FormValue("end_date")
	if appId == "" {
		rest.Error(w, "missing 'appid'", http.StatusBadRequest)
		return
	}

	var (
		start time.Time
		end   time.Time
		err   error
	)
	if startDate == "" {
		start = time.Now()
	} else {
		if start, err = time.Parse("20060102", startDate); err != nil {
			rest.Error(w, "invalid date format", http.StatusBadRequest)
			return
		}
	}
	if endDate == "" {
		end = time.Now()
	} else {
		if end, err = time.Parse("20060102", endDate); err != nil {
			rest.Error(w, "invalid date format", http.StatusBadRequest)
			return
		}
	}
	if start.After(end) {
		rest.Error(w, "start date greater than end date", http.StatusBadRequest)
		return
	}

	resp := Response{
		ErrNo: 0,
	}
	resp.Data, err = storage.Instance.GetAppStats(appId, start, end)
	if err != nil {
		rest.Error(w, "storage I/O failed", http.StatusInternalServerError)
		log.Warnf("GetAppStats failed: %s", err.Error())
		return
	}
	w.WriteJson(resp)
}

func getSysStats(w rest.ResponseWriter, r *rest.Request) {
	r.ParseForm()
	startDate := r.FormValue("start_date")
	endDate := r.FormValue("end_date")

	var (
		start time.Time
		end   time.Time
		err   error
	)
	if startDate == "" {
		start = time.Now()
	} else {
		if start, err = time.Parse("20060102", startDate); err != nil {
			rest.Error(w, "invalid date format", http.StatusBadRequest)
			return
		}
	}
	if endDate == "" {
		end = time.Now()
	} else {
		if end, err = time.Parse("20060102", endDate); err != nil {
			rest.Error(w, "invalid date format", http.StatusBadRequest)
			return
		}
	}
	if start.After(end) {
		rest.Error(w, "start date greater than end date", http.StatusBadRequest)
		return
	}

	resp := Response{
		ErrNo: 0,
	}
	resp.Data, err = storage.Instance.GetSysStats(start, end)
	if err != nil {
		rest.Error(w, "storage I/O failed", http.StatusInternalServerError)
		log.Warnf("GetSysStats failed: %s", err.Error())
		return
	}
	w.WriteJson(resp)
}
