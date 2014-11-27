package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/ant0ine/go-json-rest/rest"
	log "github.com/cihub/seelog"

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
		auth := strings.Split(authstr, " ")
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
			log.Warnf("Unknown 'Date' header: ", err)
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
	//	if !comet.DevMap.Check(devId) {
	//		rest.NotFound(w, r)
	//		return
	//	}

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

	resp := cloud.ApiResponse{}
	resp.ErrNo = cloud.ERR_NOERROR
	//resp.Data = devInfo
	w.WriteJson(resp)
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
