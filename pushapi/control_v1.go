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
// Check if the user (sso_tk) has authority to the device
//
func checkAuthz(sso_tk string, devid string) bool {
	// TODO: remove this is for test
	if sso_tk == "000000000" {
		return true
	}

	devices, err := devcenter.GetDeviceList(sso_tk, devcenter.DEV_ROUTER)
	if err != nil {
		log.Errorf("GetDeviceList failed: %s", err.Error())
		return false
	}

	for _, dev := range devices {
		if devid == dev.Id {
			return true
		}
	}
	return false
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
	sso_tk := r.FormValue("sso_tk")
	if sso_tk == "" {
		rest.Error(w, "Missing \"sso_tk\"", http.StatusBadRequest)
		return
	}

	if !checkAuthz(sso_tk, devId) {
		log.Warnf("Auth failed. sso_tk: %s, device_id: %s", sso_tk, devId)
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
		Sso_tk  string `json:"sso_tk"`
		Service string `json:"service"`
		Cmd     string `json:"cmd"`
	}

	devId := r.PathParam("devid")
	//	if !comet.DevMap.Check(devId) {
	//		rest.NotFound(w, r)
	//		return
	//	}
	param := ControlParam{}
	err := r.DecodeJsonPayload(&param)
	if err != nil {
		log.Warnf("Error decode param: %s", err.Error())
		rest.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !checkAuthz(param.Sso_tk, devId) {
		log.Warnf("Auth failed. sso_tk: %s, device_id: %s", param.Sso_tk, devId)
		rest.Error(w, "Authorization failed", http.StatusForbidden)
		return
	}

	resp := cloud.ApiResponse{}
	result, err := rpcClient.Control(devId, param.Service, param.Cmd)
	if err != nil {
		resp.ErrNo = cloud.ERR_CMD_TIMEOUT
		resp.ErrMsg = fmt.Sprintf("recv response timeout [%s]", devId)
	} else {
		resp.ErrNo = cloud.ERR_NOERROR
		resp.Data = result
	}

	w.WriteJson(resp)
}
