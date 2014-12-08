package main
import (
    "net/http"
    "encoding/json"
    "fmt"
    "os"
    "flag"
    "time"
    "bytes"
    "strings"
    "io"
    log "github.com/cihub/seelog"
    "github.com/chenyf/push/storage"
    "github.com/chenyf/push/conf"
    "github.com/chenyf/push/utils"
)

type Notice struct {
    Id int64            `json:"id"`
    AppId string        `json:"appid"`
    AppSec string       `json:"appsec"`
    Tags string         `json:"tags"`
    MsgType int         `json:"msg_type"`
    Platform string     `json:"platform,omitempty"`
    Content string      `json:"content,omitempty"`
    Notification struct {
        Title     string `json:"title"`
        Desc      string `json:"desc,omitempty"`
        Type      int    `json:"type,omitempty"`
        SoundUri  string `json:"sound_uri,omitempty"`
        Action    int    `json:"action,omitempty"`
        IntentUri string `json:"intent_uri,omitempty"`
        WebUri    string `json:"web_uri,omitempty"`
    } `json:"notification,omitempty"`
    Options struct {
        TTL int64 `json:"ttl,omitempty"`
        TTS int64 `json:"tts,omitempty"`
    } `json:"options"`
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
        Sent []ResponseItem     `json:"sent,omitempty"`
        UnSent []ResponseItem   `json:"unsent,omitempty"`
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

func callThirdPartyIf(method string, url string, body io.Reader, header *map[string]string) (error, interface{}) {
    client := &http.Client{}
    req, err := http.NewRequest(method, url, body)
    if err != nil {
        return err, nil
    }

    if header != nil {
        for k, v := range *header {
            req.Header.Set(k, v)
        }
    }
    resp, err := client.Do(req)

    if err != nil {
        return err, nil
    }

    r := ThirdPartyResponse{}
    err = json.NewDecoder(resp.Body).Decode(&r)

    if err != nil {
        return err, nil
    }
    if r.ErrMsg != "" {
        return fmt.Errorf("failed to get response data [%s]", r.ErrMsg), nil
    }
    return nil, r.Data
}

func getUids(resp interface{}, uids *[]string) {
    data := resp.(map[string]interface{})
    found := make(map[string]bool)
    for _, vdata := range data {
        _vdata := vdata.(map[string]interface{})
        for key, val := range _vdata {
            if key == "uids" {
                for _, uid := range val.([]interface{}) {
                    _uid := uid.(string)
                    if _, ok := found[_uid]; !ok {
                        found[_uid] = true
                        *uids = append(*uids, uid.(string))
                    }
                }
            }
        }
    }
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
                AppId: n.AppId,
                MsgType: n.MsgType,
                Content: n.Content,
                Platform: n.Platform,
            }
            d.Notification = n.Notification
            d.Options = n.Options
            log.Infof("get uids with notice[%d]", n.Id)
            err, resp := callThirdPartyIf("GET", fmt.Sprintf("%s%s", conf.Config.Notify.SubUrl, n.Tags), nil, nil)
            if err != nil {
                rchan <- &Result{
                    Error: err,
                    Data: nil,
                }
                return
            }
            var uids []string
            getUids(resp, &uids)
            d.PushParams.UserId = uids
            data, _ := json.Marshal(d)
            log.Infof("push msgs with notice[%d]", n.Id)
            path := strings.SplitN(conf.Config.Notify.PushUrl, "/", 4)
            date := time.Now().UTC().String()
            sign := utils.Sign(n.AppSec, "POST", fmt.Sprintf("/%s", path[3]), data, date, nil)
            header := map[string]string{
                "Content-Type": "application/json",
                "Date": date,
                "Authorization": fmt.Sprintf("LETV %s %s", n.AppId, sign),
            }
            err, resp = callThirdPartyIf("POST", conf.Config.Notify.PushUrl, bytes.NewBuffer(data), &header)
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
    err = http.ListenAndServe(conf.Config.Notify.Addr, nil)
    if err != nil {
        log.Warnf("failed to ListenAndServe: ", err)
        os.Exit(1)
    }
}
