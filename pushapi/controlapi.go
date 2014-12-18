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
	"github.com/chenyf/push/stats"
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
	stats.QueryOnlineDevices()

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

	stats.QueryDeviceInfo()

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

	stats.Cmd(param.Service)
	resp := cloud.ApiResponse{}
	result, err := rpcClient.Control(devId, param.Service, param.Cmd)
	if err != nil {
		if _, ok := err.(*mq.NoDeviceError); ok {
			stats.CmdOffline(param.Service)
			rest.NotFound(w, r)
			return
		} else if _, ok := err.(*mq.TimeoutError); ok {
			stats.CmdTimeout(param.Service)
			resp.ErrNo = cloud.ERR_CMD_TIMEOUT
			resp.ErrMsg = fmt.Sprintf("recv response timeout [%s]", devId)
		} else if _, ok := err.(*mq.InvalidServiceError); ok {
			stats.CmdInvalidService(param.Service)
			resp.ErrNo = cloud.ERR_CMD_INVALID_SERVICE
			resp.ErrMsg = fmt.Sprintf("Device [%s] has no service [%s]", devId, param.Service)
		} else if _, ok := err.(*mq.SdkError); ok {
			stats.CmdOtherError(param.Service)
			resp.ErrNo = cloud.ERR_CMD_SDK_ERROR
			resp.ErrMsg = fmt.Sprintf("Error when calling service [%s] on [%s]", param.Service, devId)
		} else {
			stats.CmdOtherError(param.Service)
			resp.ErrNo = cloud.ERR_CMD_OTHER
			resp.ErrMsg = err.Error()
		}
	} else {
		stats.CmdSuccess(param.Service)
		resp.ErrNo = cloud.ERR_NOERROR
		resp.Data = result
	}

	w.WriteJson(resp)
}

func getStats(w rest.ResponseWriter, r *rest.Request) {
	resp := cloud.ApiResponse{
		ErrNo: cloud.ERR_NOERROR,
	}
	resp.Data, _ = stats.GetStats()
	w.WriteJson(resp)
}

func deleteStats(w rest.ResponseWriter, r *rest.Request) {
	if err := stats.ClearStats(); err != nil {
		rest.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := cloud.ApiResponse{}
	resp.ErrNo = cloud.ERR_NOERROR

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

type SignMiddleware struct{}

func (this *SignMiddleware) MiddlewareFunc(handler rest.HandlerFunc) rest.HandlerFunc {
	return func(w rest.ResponseWriter, r *rest.Request) {
		if r.URL.Path == "/list" || r.URL.Path == "/command" {
			resp := CommandResponse{}
			r.ParseForm()

			uid := r.FormValue("uid")
			if uid == "" {
				resp.Status = STATUS_INVALID_PARAM
				resp.Error = "missing 'uid'"
				w.WriteJson(resp)
				return
			}

			sign := r.FormValue("sign")
			if sign == "" {
				resp.Status = STATUS_INVALID_PARAM
				resp.Error = "missing 'sign'"
				w.WriteJson(resp)
				return
			}

			tm := r.FormValue("tm")
			if tm == "" {
				resp.Status = STATUS_INVALID_PARAM
				resp.Error = "missing 'tm'"
				w.WriteJson(resp)
				return
			}

			pmtt := r.FormValue("pmtt")
			if pmtt == "" {
				resp.Status = STATUS_INVALID_PARAM
				resp.Error = "missing 'pmtt'"
				w.WriteJson(resp)
				return
			}

			src := r.FormValue("src")
			if src == "" {
				src = "src"
			}

			rid := r.FormValue("rid")
			if rid == "" {
				rid = "rid"
			}

			tid := r.FormValue("tid")
			if tid == "" {
				tid = "tid"
			}

			query := map[string]string{
				"uid":  uid,
				"rid":  rid,
				"tid":  tid,
				"src":  src,
				"tm":   tm,
				"pmtt": pmtt,
			}

			if sign != "supersign" {
				mysign := sign_calc("/router"+r.URL.Path, query)
				if mysign != sign {
					log.Warnf("sign valication failed: %s %s", mysign, sign)
					resp.Error = "sign valication failed"
					resp.Status = STATUS_INVALID_PARAM
					w.WriteJson(resp)
					return
				}
			}
		}

		handler(w, r)
	}
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

func postRouterCommand(w rest.ResponseWriter, r *rest.Request) {
	log.Debugf("Request from RemoterAddr: %s", r.RemoteAddr)

	resp := CommandResponse{}
	resp.Status = STATUS_OK

	uid := r.FormValue("uid")
	if uid == "" {
		resp.Status = STATUS_INVALID_PARAM
		resp.Error = "missing 'uid'"
		w.WriteJson(resp)
		return
	}

	rid := r.FormValue("rid")
	if rid == "" {
		resp.Status = STATUS_INVALID_PARAM
		resp.Error = "missing 'rid'"
		w.WriteJson(resp)
		return
	}

	if !checkAuthzUid(uid, rid) {
		log.Warnf("auth failed. uid: %s, rid: %s", uid, rid)
		resp.Status = STATUS_OTHER_ERR
		resp.Error = "authorization failed"
		w.WriteJson(resp)
		return
	}

	if r.Body == nil {
		resp.Status = STATUS_INVALID_PARAM
		resp.Error = "missing POST data"
		w.WriteJson(resp)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		resp.Status = STATUS_INVALID_PARAM
		resp.Error = "invalid POST body"
		w.WriteJson(resp)
		return
	}

	var cmd string
	var serviceName string

	if len(rid) == len("c80e774a1e78") {
		// To old agent
		serviceName = "gibbon_agent"
		type CommandRequest struct {
			//Uid string `json:"uid"`
			Cmd string `json:"cmd"`
		}
		cmdRequest := CommandRequest{
			//Uid: uid,
			Cmd: string(body),
		}
		bCmd, _ := json.Marshal(cmdRequest)
		cmd = string(bCmd)
	} else {
		// To new android service
		serviceName = "com.letv.letvroutersettingservice"
		type RouterCommand struct {
			Forward string `json:"forward"`
		}
		var rc RouterCommand
		if err := json.Unmarshal([]byte(body), &rc); err != nil {
			resp.Status = -2000
			resp.Error = "Request body is not JSON"
			w.WriteJson(resp)
			return
		}
		cmd = rc.Forward
		if cmd == "" {
			resp.Status = -2001
			resp.Error = "'forward' is empty"
			w.WriteJson(resp)
			return
		}
	}

	stats.Cmd(serviceName)
	result, err := rpcClient.Control(rid, serviceName, cmd)
	if err != nil {
		if _, ok := err.(*mq.NoDeviceError); ok {
			stats.CmdOffline(serviceName)
			resp.Status = STATUS_ROUTER_OFFLINE
			resp.Error = fmt.Sprintf("device (%s) offline", rid)
		} else if _, ok := err.(*mq.TimeoutError); ok {
			stats.CmdTimeout(serviceName)
			resp.Status = STATUS_OTHER_ERR
			resp.Error = fmt.Sprintf("recv response timeout [%s]", rid)
		} else if _, ok := err.(*mq.InvalidServiceError); ok {
			stats.CmdInvalidService(serviceName)
			resp.Status = STATUS_OTHER_ERR
			resp.Error = fmt.Sprintf("Device [%s] has no service [%s]", rid, serviceName)
		} else {
			stats.CmdOtherError(serviceName)
			resp.Status = STATUS_OTHER_ERR
			resp.Error = err.Error()
		}
		w.WriteJson(resp)
		return
	} else {
		stats.CmdSuccess(serviceName)
		if len(rid) == len("c80e774a1e78") {
			// reply from gibbon agent
			w.(http.ResponseWriter).Write([]byte(result))
		} else {
			// reply from android service
			type RouterCommandReply struct {
				Status int    `json:"status"`
				Descr  string `json:"descr"`
				Result string `json:"result"`
			}
			var reply RouterCommandReply
			reply.Status = 0
			reply.Descr = "OK"
			reply.Result = result
			w.WriteJson(reply)
		}
	}
}

func getRouterList(w rest.ResponseWriter, r *rest.Request) {
	type RouterInfo struct {
		Rid   string `json:"rid"`
		Rname string `json:"rname"`
	}

	type ResponseRouterList struct {
		Status int          `json:"status"`
		Descr  string       `json:"descr"`
		List   []RouterInfo `json:"list"`
	}

	resp := ResponseRouterList{}
	resp.Status = STATUS_OTHER_ERR
	r.ParseForm()

	uid := r.FormValue("uid")
	if uid == "" {
		resp.Status = STATUS_INVALID_PARAM
		resp.Descr = "missing 'uid'"
		w.WriteJson(resp)
		return
	}

	devices, err := devcenter.GetDevices(uid, devcenter.DEV_ROUTER)
	if err != nil {
		log.Errorf("GetDevices failed: %s", err.Error())
		resp.Descr = err.Error()
		w.WriteJson(resp)
		return
	}

	for _, dev := range devices {
		router := RouterInfo{
			Rid:   dev.Id,
			Rname: dev.Title,
		}
		resp.List = append(resp.List, router)
	}

	resp.Status = 0
	resp.Descr = "OK"
	w.WriteJson(resp)
}
