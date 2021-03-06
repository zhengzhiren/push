package devcenter

import (
	"encoding/json"
	"errors"
	"fmt"
	//	log "github.com/cihub/seelog"
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

func IsBinding(sso_tk, devId string) (bool, error) {
	type BindingResp struct {
		cloud.ApiStatus
		Data struct {
			BindStatus bool `json:"bindStatus"`
		} `json:"data"`
	}
	url := fmt.Sprintf("http://%s/api/v1/device/bind/%s/status?sso_tk=%s", conf.Config.Control.DevCenter, devId, sso_tk)
	res, err := http.Get(url)
	if err != nil {
		return false, err
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		res.Body.Close()
		return false, err
	}
	res.Body.Close()

	var result BindingResp
	err = json.Unmarshal(body, &result)
	if err != nil {
		return false, err
	}

	if result.ErrNo != cloud.ERR_NOERROR {
		return false, errors.New(result.ErrMsg)
	}

	return result.Data.BindStatus, nil
}

func GetDeviceList(sso_tk string, devType int) ([]Device, error) {
	url := fmt.Sprintf("http://%s/api/v1/device/bind/?sso_tk=%s&type=%d", conf.Config.Control.DevCenter, sso_tk, devType)
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		res.Body.Close()
		return nil, err
	}
	res.Body.Close()

	//log.Debugf("Got response from device center: %s", body)

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
	url := fmt.Sprintf("http://%s/api/v1/device/bind/?user_id=%s&type=%d", conf.Config.Control.DevCenter, uid, devType)
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		res.Body.Close()
		return nil, err
	}
	res.Body.Close()

	//log.Debugf("Got response from device center: %s", body)

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
