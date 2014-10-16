package auth

import (
	"fmt"
	"net/http"
	"io/ioutil"
	"encoding/json"
	"github.com/chenyf/push/conf"
	log "github.com/cihub/seelog"
)

type LetvAuth struct {
	url	string
}

type tokenResult struct {
	Bean	struct {
		Result	string		`json:"result"`
	}		`json:"bean"`
	Status  string			`json:"status"`
	ErrCode	string			`json:"errorCode"`
}

func (this *LetvAuth)Auth(token string) (bool, string) {
	url := fmt.Sprintf("%s/%s", this.url, token)
	res, err := http.Get(url)
	if err != nil {
		log.Warnf("http get failed: %s", err)
		return false, ""
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Warnf("ioutil readall failed: %s", err)
		return false, ""
	}
	var tr tokenResult
	err = json.Unmarshal(body, &tr)
	if err != nil {
		log.Warnf("json unmarshal failed: %s", err)
		return false, ""
	}
	if tr.Status != "1" || tr.ErrCode != "0" {
		log.Infof("sso result failed: (%s) (%s)", tr.Status, tr.ErrCode)
		return false, ""
	}
	return true, "letv_" + tr.Bean.Result
}

func newLetvAuth() *LetvAuth {
	return &LetvAuth{
		url : conf.Config.Auth.LetvUrl,
	}
}
