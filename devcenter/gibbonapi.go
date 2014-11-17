package devcenter

import (
	"encoding/json"
	"errors"
	"fmt"
	log "github.com/cihub/seelog"
	"io/ioutil"
	"net/http"

	"github.com/chenyf/push/cloud"
	"github.com/chenyf/push/conf"
)

const (
	DEV_ROUTER = 3
)

type Device struct {
	Id    string `json:"id"`
	Type  int    `json:"type"`
	Title string `json:"title"`
}

type devicesResult struct {
	ErrNo  int    `json:"errno"`
	ErrMsg string `json:"errmsg"`
	Data   struct {
		UserOpenId int      `json:"userOpenId"`
		DeviceList []Device `json:"device"`
	} `json:"data"`
}

func GetDeviceList(sso_tk string, devType int) ([]Device, error) {
	url := fmt.Sprintf("http://%s/api/v1/device/bind/?sso_tk=%s&type=%d", conf.Config.DevCenter, sso_tk, devType)
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	log.Debugf("Got response from device center: %s", body)

	var result devicesResult
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}

	if result.ErrNo != cloud.ERR_NOERROR {
		return nil, errors.New(result.ErrMsg)
	}

	return result.Data.DeviceList, nil
}

func GetDevices(uid string, devType int) ([]Device, error) {
	url := fmt.Sprintf("http://%s/api/v1/device/bind/?user_id=%s&type=%d", conf.Config.DevCenter, uid, devType)
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	log.Debugf("Got response from device center: %s", body)

	var result devicesResult
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}

	if result.ErrNo != cloud.ERR_NOERROR {
		return nil, errors.New(result.ErrMsg)
	}

	return result.Data.DeviceList, nil
}
