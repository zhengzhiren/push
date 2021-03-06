package main

import (
	"encoding/json"
	"flag"
	"fmt"
	log "github.com/cihub/seelog"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/chenyf/push/auth"
	"github.com/chenyf/push/comet"
	"github.com/chenyf/push/conf"
	"github.com/chenyf/push/mq"
	"github.com/chenyf/push/storage"
	"github.com/chenyf/push/utils"
	"github.com/chenyf/push/zk"
)

func deviceHandler(w http.ResponseWriter, r *http.Request) {
	devid := r.FormValue("devid")
	x := comet.DevicesMap.Get(devid)
	if x == nil {
		http.Error(w, "offline", 404)
		return
	}
	client := x.(*comet.Client)
	fmt.Fprintf(w, "devid: %s\n", devid)
	for _, regapp := range client.RegApps {
		fmt.Fprintf(w,
			"regid: %s\nappid: %s\nuserid: %s\nlast_msgid: %d\n",
			regapp.RegId,
			regapp.AppId,
			regapp.UserId,
			regapp.LastMsgId)
	}
}

type Status struct {
	DevCnt    int `json:"devcnt"`
	RegAppCnt int `json:"regappcnt"`
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	s := Status{
		DevCnt:    comet.DevicesMap.Size(),
		RegAppCnt: comet.AMInstance.GetCount(),
	}
	b, _ := json.Marshal(&s)
	fmt.Fprintf(w, "%s", b)
}

func main() {
	var (
		//flRoot               = flag.String("g", "/tmp/echoserver", "Path to use as the root of the docker runtime")
		//flDebug		= flag.Bool("D", false, "Enable debug mode")
		//flTest		= flag.Bool("t", false, "Enable test mode, no rabbitmq")
		flConfig = flag.String("c", "./conf/conf.json", "Config file")
	)
	flag.Parse()
	config_file := "./conf/conf.json"
	if flConfig != nil {
		config_file = *flConfig
	}
	err := conf.LoadConfig(config_file)
	if err != nil {
		fmt.Printf("LoadConfig (%s) failed: (%s)\n", config_file, err)
		os.Exit(1)
	}

	err = log.RegisterCustomFormatter("Ms", utils.CreateMsFormatter)
	if err != nil {
		fmt.Printf("Failed to create custom formatter: (%s)\n", err)
		os.Exit(1)
	}

	logger, err := log.LoggerFromConfigAsFile("./conf/log.xml")
	if err != nil {
		fmt.Printf("Load log config failed: (%s)\n", err)
		os.Exit(1)
	}

	log.ReplaceLogger(logger)

	log.Infof("pushd starting...")
	storage.NewInstance(&conf.Config)
	auth.NewInstance(&conf.Config)

	var ato uint32 = uint32(comet.ACCEPT_TIMEOUT)
	if conf.Config.Comet.AcceptTimeout > 0 {
		ato = conf.Config.Comet.AcceptTimeout
	}
	var rto uint32 = 60
	if conf.Config.Comet.ReadTimeout > 0 {
		rto = conf.Config.Comet.ReadTimeout
	}
	var wto uint32 = 60
	if conf.Config.Comet.WriteTimeout > 0 {
		wto = conf.Config.Comet.WriteTimeout
	}
	var hto uint32 = 240
	if conf.Config.Comet.HeartbeatTimeout > 0 {
		hto = conf.Config.Comet.HeartbeatTimeout
	}
	var mbl uint32 = 2048
	if conf.Config.Comet.MaxBodyLen > 0 {
		mbl = conf.Config.Comet.MaxBodyLen
	}
	var mc uint32 = 10000
	if conf.Config.Comet.MaxClients > 0 {
		mc = conf.Config.Comet.MaxClients
	}
	var worker int = 100
	if conf.Config.Comet.WorkerCnt > 0 {
		worker = conf.Config.Comet.WorkerCnt
	}
	cometServer := comet.NewServer(ato, rto, wto, hto, mbl, mc, worker)

	if conf.Config.Comet.HeartbeatInterval > 0 {
		cometServer.HbInterval = conf.Config.Comet.HeartbeatInterval
	}
	if conf.Config.Comet.ReconnTime > 0 {
		cometServer.ReconnTime = conf.Config.Comet.ReconnTime
	}

	listener, err := cometServer.Init("0.0.0.0:" + conf.Config.Comet.Port)
	if err != nil {
		log.Critical(err)
		os.Exit(1)
	}

	var mqConsumer *mq.Consumer = nil
	if conf.Config.Rabbit.Enable {
		mqConsumer, err = mq.NewConsumer(
			conf.Config.Rabbit.Uri,
			conf.Config.Rabbit.Exchange,
			conf.Config.Rabbit.QOS)
		if err != nil {
			log.Critical(err)
			os.Exit(1)
		}
	}

	if conf.Config.ZooKeeper.Enable {
		if err = zk.InitZk(&conf.Config); err != nil {
			log.Critical(err)
			os.Exit(1)
		}
	}

	wg := &sync.WaitGroup{}
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		sig := <-c
		log.Infof("Received signal '%v', exiting\n", sig)
		///utils.RemovePidFile(srv.	runtime.config.Pidfile)
		cometServer.Stop()
		log.Infof("leave 1")
		if conf.Config.Rabbit.Enable {
			mqConsumer.Shutdown()
		}
		wg.Done()
		log.Infof("leave 2")
	}()

	go func() {
		cometServer.Run(listener)
	}()

	if conf.Config.Rabbit.Enable {
		go func() {
			log.Infof("mq running")
			mqConsumer.Consume()
		}()
	}
	wg.Add(1)
	go func() {
		http.HandleFunc("/status", statusHandler)
		http.HandleFunc("/device", deviceHandler)
		err := http.ListenAndServe(conf.Config.Web, nil)
		if err != nil {
			log.Critical("http listen: ", err)
			os.Exit(1)
		}
	}()

	if conf.Config.Prof {
		go func() {
			http.ListenAndServe(":6789", nil)
		}()
	}

	_, err = mq.NewRpcServer(conf.Config.Rabbit.Uri, "gibbon_rpc_exchange", cometServer.Name)
	if err != nil {
		log.Critical("failed to start RPC server: ", err)
		os.Exit(1)
	}

	log.Infof("pushd started")
	wg.Wait()
}
