package auth

import (
	"fmt"
	"net/http"
	"io/ioutil"
	"encoding/json"
)

type tokenResult struct {
	bean	struct {
		result	string		`json:"result"`
	}		`json:"bean"`
	status	string			`json:"status"`
	errcode	string			`json:"errorCode"`
}

func CheckAuth(token string) (bool, string) {
	url := fmt.Sprintf("http://api.sso.letv.com/api/checkTicket/tk/%s", token)
	res, err := http.Get(url)
	if err != nil {
		return false, ""
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return false, ""
	}
	var tr tokenResult
	err = json.Unmarshal(body, &tr)
	if err != nil {
		return false, ""
	}
	if tr.status != "1" || tr.errcode != "0" {
		return false, ""
	}
	return true, tr.bean.result
}

