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

func TestPushSign(t *testing.T) {
	param := map[string]interface{}{
		"msg_type":  1,
		"push_type": 1,
		"content":   "just a test",
	}

	accessKey := "appid_b515357337f7415ab9275df7a3f92d94"
	secretKey := "appsec_ckeasUHYFkAvEitqagAr"
	date := time.Now().Format(time.RFC1123)
	body, _ := json.Marshal(param)
	req, _ := http.NewRequest("POST", "http://127.0.0.1:8080/api/v1/message", bytes.NewBuffer(body))
	sign := utils.Sign(secretKey, "POST", req.URL.Path, body, date, nil)
	//req, _ := http.NewRequest("POST", "http://push.scloud.letv.com/api/v1/message", bytes.NewBuffer(body))
	req.Header.Add("Date", date)
	req.Header.Add("Authorization", fmt.Sprintf("LETV %s %s", accessKey, sign))

	reqDump, _ := httputil.DumpRequest(req, true)
	t.Logf("HTTP Request:\n%s", reqDump)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Errorf("request err: %s", err)
		t.FailNow()
	}

	respDump, _ := httputil.DumpResponse(resp, true)
	t.Logf("HTTP Response:\n%s", respDump)
	if resp.StatusCode != http.StatusOK {
		t.FailNow()
	}
}

func TestControlSign(t *testing.T) {
	param := map[string]interface{}{
		"token":   "000000000",
		"service": "service1",
		"cmd":     "reboot",
	}

	devId := "device1"
	accessKey := "820b4376bad3486199e13a7ada104106"
	secretKey := "EwYmYyqdChgitRcrInBg"
	date := time.Now().Format(time.RFC1123)
	body, _ := json.Marshal(param)
	req, _ := http.NewRequest("POST", fmt.Sprintf("http://127.0.0.1:8080/api/v1/devices/%s", devId), bytes.NewBuffer(body))
	sign := utils.Sign(secretKey, "POST", req.URL.Path, body, date, nil)
	//req, _ := http.NewRequest("POST", "http://push.scloud.letv.com/api/v1/message", bytes.NewBuffer(body))
	req.Header.Add("Date", date)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("LETV %s %s", accessKey, sign))

	reqDump, _ := httputil.DumpRequest(req, true)
	t.Logf("HTTP Request:\n%s", reqDump)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Errorf("request err: %s", err)
		t.FailNow()
	}

	respDump, _ := httputil.DumpResponse(resp, true)
	t.Logf("HTTP Response:\n%s", respDump)
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusBadRequest {
		t.FailNow()
	}
}
