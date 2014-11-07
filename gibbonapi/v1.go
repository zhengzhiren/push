package main

import (
	"encoding/json"
	"fmt"
	log "github.com/cihub/seelog"
	"net/http"
	"time"

	"github.com/ant0ine/go-json-rest/rest"
	"github.com/chenyf/gibbon/cloud"
	"github.com/chenyf/gibbon/comet"
	"github.com/chenyf/gibbon/devcenter"
	"github.com/chenyf/gibbon/zk"
)

var (
	commandTimeout int
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
	resp.Data = fmt.Sprintf("Total registered devices: %d", comet.DevMap.Size())
	w.WriteJson(resp)
}

func getGibbon(w rest.ResponseWriter, r *rest.Request) {
	resp := cloud.ApiResponse{}
	resp.ErrNo = cloud.ERR_NOERROR
	resp.Data = zk.GetComet()
	w.WriteJson(resp)
}

func getDevInfo(client *comet.Client) devInfo {
	devInfo := devInfo{
		Id:        client.DevId,
		LastAlive: client.LastAlive.String(),
		RegTime:   client.RegistTime.String(),
	}
	return devInfo
}

func getDeviceList(w rest.ResponseWriter, r *rest.Request) {
	devInfoList := []devInfo{}
	devMap := comet.DevMap.Items()
	for _, client := range devMap {
		devInfoList = append(devInfoList, getDevInfo(client.(*comet.Client)))
	}

	resp := cloud.ApiResponse{}
	resp.ErrNo = cloud.ERR_NOERROR
	resp.Data = devInfoList
	w.WriteJson(resp)
}

func getDevice(w rest.ResponseWriter, r *rest.Request) {
	devId := r.PathParam("devid")
	if !comet.DevMap.Check(devId) {
		rest.NotFound(w, r)
		return
	}

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

	client := comet.DevMap.Get(devId).(*comet.Client)
	devInfo := getDevInfo(client)

	resp := cloud.ApiResponse{}
	resp.ErrNo = cloud.ERR_NOERROR
	resp.Data = devInfo
	w.WriteJson(resp)
}

func controlDevice(w rest.ResponseWriter, r *rest.Request) {
	type ControlParam struct {
		Sso_tk  string `json:"sso_tk"`
		Service string `json:"service"`
		Cmd     string `json:"cmd"`
	}

	devId := r.PathParam("devid")
	if !comet.DevMap.Check(devId) {
		rest.NotFound(w, r)
		return
	}
	client := comet.DevMap.Get(devId).(*comet.Client)
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
	result, err := control(param.Service, param.Cmd)
	if err != nil {
		resp.ErrNo = cloud.ERR_CMD_TIMEOUT
		resp.ErrMsg = fmt.Sprintf("recv response timeout [%s]", client.DevId)
	} else {
		resp.ErrNo = cloud.ERR_NOERROR
		resp.Data = result
	}

	w.WriteJson(resp)
}
