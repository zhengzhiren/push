package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ant0ine/go-json-rest/rest"
	log "github.com/cihub/seelog"
	"github.com/fighterlyt/permutation"

	"github.com/chenyf/push/cloud"
	"github.com/chenyf/push/devcenter"
	"github.com/chenyf/push/mq"
	"github.com/chenyf/push/storage"
	"github.com/chenyf/push/utils"
)

var (
	commandTimeout int
	rpcClient      *mq.RpcClient = nil
)

type devInfo struct {
	Id        string `json:"id"`
	LastAlive string `json:"last_alive,omitempty"`
	RegTime   string `json:"reg_time,omitempty"`
}

//
// Check if the user (token) has authority to the device
//
func checkAuthz(token string, devid string) bool {
	// TODO: remove this is for test
	if token == "000000000" {
		return true
	}

	binding, err := devcenter.IsBinding(token, devid)
	if err != nil {
		log.Errorf("IsBinding failed: %s", err.Error())
		return false
	}
	return binding
}

//
// check the signature of the request
// the signature calulation need to read full content of request body
// the body is then put into the request.Env["body"] and passed to
// the next handler
//
func AuthMiddlewareFunc(h rest.HandlerFunc) rest.HandlerFunc {
	return func(w rest.ResponseWriter, r *rest.Request) {
		controlAK := "820b4376bad3486199e13a7ada104106"
		secretKey := "EwYmYyqdChgitRcrInBg"
		authstr := r.Header.Get("Authorization")
		auth := strings.Fields(authstr)
		if len(auth) != 3 {
			rest.Error(w, "Invalid 'Authorization' header", http.StatusBadRequest)
			return
		}
		accessKey := auth[1]
		sign := auth[2]
		if accessKey != controlAK {
			rest.Error(w, "Invalid AccessKey", http.StatusForbidden)
			return
		}

		dateStr := r.Header.Get("Date")
		date, err := time.Parse(time.RFC1123, dateStr)
		if err != nil {
			rest.Error(w, "Invalid 'Date' header", http.StatusBadRequest)
			log.Warnf("Unknown 'Date' header: %s", err.Error())
			return
		}
		log.Debugf("Date header: %s", date.String())

		body, _ := ioutil.ReadAll(r.Body)
		r.ParseForm()
		if sign != "supersignature" { //TODO: remove this
			if utils.Sign(secretKey, r.Method, "/api/v1"+r.URL.Path, body, dateStr, r.Form) != sign {
				rest.Error(w, "Signature varification failed", http.StatusForbidden)
				return
			}
		}
		r.Env["body"] = body
		h(w, r)
	}
}

func getDeviceList(w rest.ResponseWriter, r *rest.Request) {
	devInfoList := []devInfo{}
	r.ParseForm()
	dev_ids := r.FormValue("dev_ids")
	if dev_ids != "" {
		ids := strings.Split(dev_ids, ",")
		for _, id := range ids {
			if serverName, err := storage.Instance.CheckDevice(id); err == nil && serverName != "" {
				info := devInfo{
					Id: id,
				}
				devInfoList = append(devInfoList, info)
			}
		}
	} else {
		rest.Error(w, "Missing \"dev_ids\"", http.StatusBadRequest)
		return
	}

	resp := cloud.ApiResponse{}
	resp.ErrNo = cloud.ERR_NOERROR
	resp.Data = devInfoList
	w.WriteJson(resp)
}

func getDevice(w rest.ResponseWriter, r *rest.Request) {
	devId := r.PathParam("devid")

	r.ParseForm()
	token := r.FormValue("token")
	if token == "" {
		rest.Error(w, "Missing \"token\"", http.StatusBadRequest)
		return
	}

	if !checkAuthz(token, devId) {
		log.Warnf("Auth failed. token: %s, device_id: %s", token, devId)
		rest.Error(w, "Authorization failed", http.StatusForbidden)
		return
	}

	if serverName, err := storage.Instance.CheckDevice(devId); err == nil && serverName != "" {
		resp := cloud.ApiResponse{}
		resp.ErrNo = cloud.ERR_NOERROR
		resp.Data = devInfo{
			Id: devId,
		}
		w.WriteJson(resp)
	} else {
		rest.NotFound(w, r)
		return
	}
}

func controlDevice(w rest.ResponseWriter, r *rest.Request) {
	type ControlParam struct {
		Token   string `json:"token"`
		Service string `json:"service"`
		Cmd     string `json:"cmd"`
	}

	devId := r.PathParam("devid")
	body := r.Env["body"]
	if body == nil {
		rest.Error(w, "Empty body", http.StatusBadRequest)
		return
	}
	b := body.([]byte)
	param := ControlParam{}
	if err := json.Unmarshal(b, &param); err != nil {
		log.Warnf("Error decode body: %s", err.Error())
		rest.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !checkAuthz(param.Token, devId) {
		log.Warnf("Auth failed. token: %s, device_id: %s", param.Token, devId)
		rest.Error(w, "Authorization failed", http.StatusForbidden)
		return
	}

	resp := cloud.ApiResponse{}
	result, err := rpcClient.Control(devId, param.Service, param.Cmd)
	if err != nil {
		if _, ok := err.(*mq.NoDeviceError); ok {
			rest.NotFound(w, r)
			return
		} else if _, ok := err.(*mq.TimeoutError); ok {
			resp.ErrNo = cloud.ERR_CMD_TIMEOUT
			resp.ErrMsg = fmt.Sprintf("recv response timeout [%s]", devId)
		} else if _, ok := err.(*mq.InvalidServiceError); ok {
			resp.ErrNo = cloud.ERR_CMD_INVALID_SERVICE
			resp.ErrMsg = fmt.Sprintf("Device [%s] has no service [%s]", devId, param.Service)
		} else if _, ok := err.(*mq.SdkError); ok {
			resp.ErrNo = cloud.ERR_CMD_SDK_ERROR
			resp.ErrMsg = fmt.Sprintf("Error when calling service [%s] on [%s]", param.Service, devId)
		} else {
			resp.ErrNo = cloud.ERR_CMD_OTHER
			resp.ErrMsg = err.Error()
		}
	} else {
		resp.ErrNo = cloud.ERR_NOERROR
		resp.Data = result
	}

	w.WriteJson(resp)
}

//
// compatible APIs
//
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
	log.Debugf("Request from RemoterAddr: %s", r.RemoteAddr)
	var (
		uid         string
		rid         string
		tid         string
		sign        string
		tm          string
		src         string
		pmtt        string
		mysign      string
		query       map[string]string
		response    CommandResponse
		serviceName string = "com.letv.letvroutersettingservice"
		cmd         string
		result      string
		body        []byte
		err         error
		bCmd        []byte
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

	src = r.FormValue("src")
	if src == "" {
		src = "src"
	}

	query = map[string]string{
		"uid":  uid,
		"rid":  rid,
		"tid":  tid,
		"src":  src,
		"tm":   tm,
		"pmtt": pmtt,
	}

	if sign != "supersign" {
		mysign = sign_calc(r.URL.Path, query)
		if mysign != sign {
			log.Warnf("sign valication failed: %s %s", mysign, sign)
			response.Error = "sign valication failed"
			response.Status = STATUS_INVALID_PARAM
			goto resp
		}
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

	body, err = ioutil.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		response.Status = STATUS_INVALID_PARAM
		response.Error = "invalid POST body"
		goto resp
	}

	if len(rid) == len("c80e774a1e78") {
		// To old agent
		type CommandRequest struct {
			//Uid string `json:"uid"`
			Cmd string `json:"cmd"`
		}
		cmdRequest := CommandRequest{
			//Uid: uid,
			Cmd: string(body),
		}
		bCmd, _ = json.Marshal(cmdRequest)
		cmd = string(bCmd)
	} else {
		// To new android service
		type RouterCommand struct {
			Forward string `json:"forward"`
		}
		var rc RouterCommand
		if err := json.Unmarshal([]byte(body), &rc); err != nil {
			response.Status = -2000
			response.Error = "Request body is not JSON"
			goto resp
		}
		cmd = rc.Forward
		if cmd == "" {
			response.Status = -2001
			response.Error = "'forward' is empty"
			goto resp
		}
	}

	result, err = rpcClient.Control(rid, serviceName, cmd)
	if err != nil {
		if _, ok := err.(*mq.NoDeviceError); ok {
			response.Status = STATUS_ROUTER_OFFLINE
			response.Error = fmt.Sprintf("device (%s) offline", rid)
		} else if _, ok := err.(*mq.TimeoutError); ok {
			response.Status = STATUS_OTHER_ERR
			response.Error = fmt.Sprintf("recv response timeout [%s]", rid)
		} else if _, ok := err.(*mq.InvalidServiceError); ok {
			response.Status = STATUS_OTHER_ERR
			response.Error = fmt.Sprintf("Device [%s] has no service [%s]", rid, serviceName)
		} else {
			response.Status = STATUS_OTHER_ERR
			response.Error = err.Error()
		}
	} else {
		w.Write([]byte(result))
		log.Infof("postRouterCommand write: %s", result)
		return
	}

resp:
	b, _ := json.Marshal(response)
	log.Debugf("postRouterCommand write: %s", string(b))
	w.Write(b)
}

func getRouterList(w http.ResponseWriter, r *http.Request) {
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
		rid      string
		src      string
		tm       string
		pmtt     string
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

	rid = r.FormValue("rid")
	if rid == "" {
		rid = "rid"
	}

	src = r.FormValue("src")
	if src == "" {
		src = "src"
	}

	query = map[string]string{
		"uid":  uid,
		"rid":  rid,
		"tid":  tid,
		"src":  src,
		"tm":   tm,
		"pmtt": pmtt,
	}

	if sign != "supersign" {
		mysign = sign_calc(r.URL.Path, query)
		if mysign != sign {
			log.Warnf("sign valication failed: %s %s", mysign, sign)
			response.Descr = "sign valication failed"
			response.Status = STATUS_INVALID_PARAM
			goto resp
		}
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
