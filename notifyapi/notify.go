package main
import (
    "net/http"
    "encoding/json"
    "fmt"
    "os"
    "flag"
    "time"
    "bytes"
    log "github.com/cihub/seelog"
    "github.com/chenyf/push/storage"
    "github.com/chenyf/push/conf"
)

type Notice struct {
    Id int64            `json:"id"`
    AppId string        `json:"appid"`
    Tags string         `json:"tags"`
    MsgType int         `json:"msg_type"`
    Platform string     `json:"platform"`
    Content string      `json:"content"`
    TTL int64           `json:"ttl,omitempty"`
    Token string        `json:"token"`
}

type PostNotifyData struct {
    Notices []Notice     `json:"notices"`
}

type ResponseItem struct {
    Id int64            `json:"id"`
    Data interface{}    `json:"data"`
}

type Response struct {
    ErrNo int               `json:"errno"`
    ErrMsg string           `json:"errmsg,omitempty"`
    Data struct {
        Sent []ResponseItem     `json:"sent"`
        UnSent []ResponseItem   `json:"unsent"`
    }                       `json:"data,omitempty"`
}

type Result struct {
    Error error         `json:"error"`
    Data interface{}    `json:"data"`
}

type ThirdPartyResponse struct {
    ErrNo int                   `json:"errno"`
    ErrMsg string               `json:"errmsg"`
    Data interface{}            `json:"data"`
}

const PUSH_WITH_UID = 3

func callThirdPartyIf(method string, url string, data []byte) (error, interface{}) {
    var (
        r *http.Response
        err error
    )
    if method == "GET" {
        r, err = http.Get(url)
    } else if method == "POST" {
        r, err = http.Post(url, "application/json", bytes.NewBuffer(data))
    } else {
        return fmt.Errorf("unknow http method with thirdparty interface"), nil
    }

    if err != nil {
        return err, nil
    }

    resp := ThirdPartyResponse{}
    err = json.NewDecoder(r.Body).Decode(&resp);

    if err != nil {
        return err, nil
    }
    if resp.ErrMsg != "" {
        return fmt.Errorf("failed to get response data [%s]", resp.ErrMsg), nil
    }
    return nil, resp.Data
}

func postNotify(w http.ResponseWriter, r *http.Request) {
    var response Response
    response.ErrNo = 10000
    if r.Method != "POST" {
        http.Error(w, "Method not allowed", 405)
        return
    }
    data := PostNotifyData{}
    if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
        response.ErrNo = 10001
        response.ErrMsg = "invalid notification data"
        b, _ := json.Marshal(response)
        fmt.Fprintf(w, string(b))
        return
    }

    if len(data.Notices) > conf.Config.Notify.MaxNotices {
        response.ErrNo = 10002
        response.ErrMsg = "notification msgs over limit"
        b, _ := json.Marshal(response)
        fmt.Fprintf(w, string(b))
        return
    }

    results := make(map[int64]chan *Result)
    for _, n := range data.Notices {
        log.Infof("send notice[%d]", n.Id)
        rchan := make(chan *Result)
        results[n.Id] = rchan
        go func(n Notice) {
            d := storage.RawMessage{
                PushType: PUSH_WITH_UID,
                Token: n.Token,
                AppId: n.AppId,
                MsgType: n.MsgType,
                Content: n.Content,
                Platform: n.Platform,
            }
            if n.TTL != 0 {
                d.Options.TTL = n.TTL
            }
            log.Infof("get uids with notice[%d]", n.Id)
            err, resp := callThirdPartyIf("GET", fmt.Sprintf("%s?tids=%s", conf.Config.Notify.SubUrl, n.Tags), nil)
            if err != nil {
                rchan <- &Result{
                    Error: err,
                    Data: nil,
                }
                return
            }
            rdata := resp.(map[string]interface{})
            var uids []string
            for _, i := range rdata["uids"].([]interface{}) {
                uids = append(uids, i.(string))
            }
            d.PushParams.UserId = uids
            data, _ := json.Marshal(d)
            log.Infof("push msgs with notice[%d]", n.Id)
            err, resp = callThirdPartyIf("POST", conf.Config.Notify.PushUrl, data)
            rchan <- &Result{
                Error: err,
                Data: resp,
            }
        }(n)
    }

    for nid, c := range results {
        select {
            case result := <- c:
                log.Infof("sent notice[%d]", nid)
                if result.Error != nil {
                    response.Data.UnSent = append(response.Data.UnSent, ResponseItem{Id:nid, Data:fmt.Sprintf("%s", result.Error)})
                } else {
                    response.Data.Sent = append(response.Data.Sent, ResponseItem{Id:nid, Data:result.Data})
                }
            case <-time.After(conf.Config.Notify.Timeout*time.Second):
                log.Infof("notify timeout %d", nid)
                response.Data.UnSent = append(response.Data.UnSent, ResponseItem{Id:nid, Data:fmt.Sprintf("notify timeout")})
        }
    }
    b, _ := json.Marshal(response)
    fmt.Fprintf(w, string(b))
}

func main() {
    logConfigFile := flag.String("l", "./conf/log.xml", "Log config file")
    configFile := flag.String("c", "./conf/conf.json", "Config file")

    flag.Parse()

	logger, err := log.LoggerFromConfigAsFile(*logConfigFile)
	if err != nil {
		fmt.Printf("Load log config failed: (%s)\n", err)
		os.Exit(1)
	}

	log.ReplaceLogger(logger)

    err = conf.LoadConfig(*configFile)
    if err != nil {
        log.Warnf("LoadConfig (%s) failed: (%s)\n", *configFile, err)
        os.Exit(1)
    }

    http.HandleFunc("/api/v1/notify", postNotify)
    err = http.ListenAndServe(":9999", nil)
    if err != nil {
        log.Warnf("failed to ListenAndServe: ", err)
		os.Exit(1)
    }
}
