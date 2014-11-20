package main

import (
	"fmt"
	log "github.com/cihub/seelog"
	"net/http"

	"github.com/ant0ine/go-json-rest/rest"

	"github.com/chenyf/push/cloud"
	"github.com/chenyf/push/devcenter"
	"github.com/chenyf/push/mq"
)

var (
	commandTimeout int
	rpcClient      *mq.RpcClient = nil
)

type devInfo struct {
	Id        string `json:"id"`
	LastAlive string `json:"last_alive"`
	RegTime   string `json:"reg_time"`
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

func getStatus(w rest.ResponseWriter, r *rest.Request) {
	resp := cloud.ApiResponse{}
	resp.ErrNo = cloud.ERR_NOERROR
	//resp.Data = fmt.Sprintf("Total registered devices: %d", comet.DevMap.Size())
	w.WriteJson(resp)
}

func getDeviceList(w rest.ResponseWriter, r *rest.Request) {
	devInfoList := []devInfo{}

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
	param := ControlParam{}
	err := r.DecodeJsonPayload(&param)
	if err != nil {
		log.Warnf("Error decode param: %s", err.Error())
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
