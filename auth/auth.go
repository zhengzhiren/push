package auth

import (
	"fmt"
	"net/http"
	"io/ioutil"
	"encoding/json"
	log "github.com/cihub/seelog"
)

type tokenResult struct {
	Bean	struct {
		Result	string		`json:"result"`
	}		`json:"bean"`
	Status  string			`json:"status"`
	ErrCode	string			`json:"errorCode"`
}

func CheckAuth(token string) (bool, string) {
	url := fmt.Sprintf("http://api.sso.letv.com/api/checkTicket/tk/%s", token)
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

