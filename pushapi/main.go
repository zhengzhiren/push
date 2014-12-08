package main

import (
	"encoding/json"
	"flag"
	"fmt"
	log "github.com/cihub/seelog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/ant0ine/go-json-rest/rest"

	"github.com/chenyf/push/auth"
	"github.com/chenyf/push/conf"
	"github.com/chenyf/push/mq"
	"github.com/chenyf/push/storage"
	"github.com/chenyf/push/utils"
	"github.com/chenyf/push/zk"
)

func main() {
	var (
		configFile    = flag.String("c", "./conf/conf.json", "Config file")
		logConfigFile = flag.String("l", "./conf/log.xml", "Log config file")
	)

	flag.Parse()

	err := conf.LoadConfig(*configFile)
	if err != nil {
		fmt.Printf("LoadConfig (%s) failed: (%s)\n", *configFile, err)
		os.Exit(1)
	}

	err = log.RegisterCustomFormatter("Ms", utils.CreateMsFormatter)
	if err != nil {
		fmt.Printf("Failed to create custom formatter: (%s)\n", err)
		os.Exit(1)
	}

	logger, err := log.LoggerFromConfigAsFile(*logConfigFile)
	if err != nil {
		fmt.Printf("Load log config failed: (%s)\n", err)
		os.Exit(1)
	}

	log.ReplaceLogger(logger)

	storage.NewInstance(&conf.Config)
	auth.NewInstance(&conf.Config)

	setMsgID()
	setPappID()
	mqProducer, err := mq.NewProducer(
		conf.Config.Rabbit.Uri,
		conf.Config.Rabbit.Exchange,
		conf.Config.Rabbit.ExchangeType,
		conf.Config.Rabbit.Key,
		false)
	if err != nil {
		log.Warnf("new mq produccer failed: %s", err)
		os.Exit(1)
	}
	err = zk.InitWatcher(
		conf.Config.ZooKeeper.Addr,
		conf.Config.ZooKeeper.Timeout*time.Second,
		conf.Config.ZooKeeper.Path)
	if err != nil {
		log.Warnf("init zk watcher failed: %s", err)
		os.Exit(1)
	}
	rpcClient, err = mq.NewRpcClient(conf.Config.Rabbit.Uri, "gibbon_rpc_exchange")
	if err != nil {
		log.Criticalf("Create RPC client failed: %s", err)
		os.Exit(1)
	}

	wg := &sync.WaitGroup{}
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		sig := <-c
		log.Infof("Received signal '%v', exiting\n", sig)
		close(msgBox)
		mqProducer.Shutdown()
		rpcClient.Close()
		wg.Done()
	}()

	wg.Add(1)
	go startHttp(conf.Config.PushAPI, conf.Config.CommandTimeout)

	for {
		select {
		case m, ok := <-msgBox:
			if !ok {
				os.Exit(0)
			}

			v, err := json.Marshal(m)
			if err != nil {
				log.Infof("failed to encode with Msg:", err)
				continue
			}

			if m.Options.TTL < 0 { // send immediatly
				m.Options.TTL = 0
			} else if m.Options.TTL == 0 {
				m.Options.TTL = 86400 // default
			}

			if _, err := storage.Instance.HashSet(
				"db_msg_"+m.AppId,
				strconv.FormatInt(m.MsgId, 10), v); err != nil {
				log.Infof("failed to put Msg into redis:", err)
				continue
			}

			if m.Options.TTL > 0 {
				_, err = storage.Instance.HashSet(
					"db_offline_msg_"+m.AppId,
					fmt.Sprintf("%v_%v", m.MsgId, m.Options.TTL+m.CTime), v)
				if err != nil {
					log.Infof("failed to put offline Msg into redis:", err)
					continue
				}
			}

			d := map[string]interface{}{
				"appid": m.AppId,
				"msgid": m.MsgId,
			}
			data, err := json.Marshal(d)
			if err != nil {
				log.Infof("failed to jsonencode with data:", err)
				continue
			}

			if err := mqProducer.Publish(data); err != nil {
				log.Infof("failed to publish data:", err)
				continue
			}
		}
	}
	wg.Wait()
}

func startHttp(addr string, cmdTimeout int) {
	log.Infof("Starting HTTP server on %s, command timeout: %ds", addr, cmdTimeout)
	commandTimeout = cmdTimeout

	handler := rest.ResourceHandler{
		DisableXPoweredBy:        true,
		DisableJsonIndent:        true,
		EnableStatusService:      true,
		EnableResponseStackTrace: true,
	}
	err := handler.SetRoutes(
		&rest.Route{"GET", "/devices", getDeviceList},
		&rest.Route{"GET", "/devices/:devid", getDevice},
		&rest.Route{"POST", "/devices/:devid", AuthMiddlewareFunc(controlDevice)},
		&rest.Route{"GET", "/.status",
			func(w rest.ResponseWriter, r *rest.Request) {
				w.WriteJson(handler.GetStatus())
			},
		},
	)
	if err != nil {
		log.Criticalf("http SetRoutes: ", err)
		os.Exit(1)
	}

	err = initPermutation()
	if err != nil {
		log.Criticalf("init permutation: ", err)
		os.Exit(1)
	}

	// control API
	http.Handle("/api/v1/", http.StripPrefix("/api/v1", &handler))

	// the adapter API for old system
	http.HandleFunc("/router/command", postRouterCommand)
	http.HandleFunc("/router/list", getRouterList)

	// push API
	http.HandleFunc("/api/v1/message", messageHandler)
	http.HandleFunc("/api/v1/server", serverHandler)
	http.HandleFunc("/api/v1/app", appHandler)
	http.HandleFunc("/test/message/confirm", testHandler)

	err = http.ListenAndServe(addr, nil)
	if err != nil {
		log.Criticalf("http listen: ", err)
		os.Exit(1)
	}
}
