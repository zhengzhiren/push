package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"testing"
	"time"

	"github.com/chenyf/push/utils"
)

func TestSign(t *testing.T) {
	param := map[string]interface{}{
		"msg_type":  1,
		"push_type": 1,
		"content":   "just a test",
	}

	appId := "appid_b515357337f7415ab9275df7a3f92d94"
	appSec := "appsec_ckeasUHYFkAvEitqagAr"
	date := time.Now().Format(time.RFC1123)
	body, _ := json.Marshal(param)
	sign := utils.Sign(appSec, "POST", body, date, nil)
	req, _ := http.NewRequest("POST", "http://127.0.0.1:8080/api/v1/message", bytes.NewBuffer(body))
	//req, _ := http.NewRequest("POST", "http://push.scloud.letv.com/api/v1/message", bytes.NewBuffer(body))
	req.Header.Add("Date", date)
	req.Header.Add("Authorization", fmt.Sprintf("LETV %s %s", appId, sign))

	reqDump, _ := httputil.DumpRequest(req, true)
	t.Logf("HTTP Request:\n%s", reqDump)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Errorf("request err: %s", err)
	}

	respDump, _ := httputil.DumpResponse(resp, true)
	t.Logf("HTTP Response:\n%s", respDump)
	if resp.StatusCode != http.StatusOK {
		t.FailNow()
	}
}
