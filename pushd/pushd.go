package main

import (
	"flag"
	"os"
	//"time"
	"crypto/hmac"
	"crypto/sha1"
	"fmt"
	"github.com/chenyf/push/auth"
	"github.com/chenyf/push/comet"
	"github.com/chenyf/push/conf"
	"github.com/chenyf/push/mq"
	"github.com/chenyf/push/storage"
	"github.com/chenyf/push/zk"
	log "github.com/cihub/seelog"
	"net/http"
	"os/signal"
	"strings"
	"sync"
	"syscall"
)

func deviceHandler(w http.ResponseWriter, r *http.Request) {
	devid := r.FormValue("devid")
	client := comet.DevicesMap.Get(devid).(*comet.Client)
	if client == nil {
		http.Error(w, "offline", 404)
		return
	}
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

func statusHandler(w http.ResponseWriter, r *http.Request) {
	size := comet.DevicesMap.Size()
	fmt.Fprintf(w, "total online device: %d\n", size)
}

func checkAuthz(uid string, devid string) bool {
	return false
}

func sign(path string, query map[string]string) []byte {
	uid := query["uid"]
	rid := query["rid"]
	tid := query["tid"]
	src := query["src"]
	tm := query["tm"]
	pmtt := query["pmtt"]

	raw := []string{path, uid, rid, tid, src, tm, pmtt}
	args := []string{}
	x := []int{6, 5, 4, 3, 2, 1, 0}
	for _, item := range x {
		args = append(args, raw[item])
	}
	data := strings.Join(args, "")
	key := "xnRzFxoCDRVRU2mNQ7AoZ5MCxpAR7ntnmlgRGYav"
	mac := hmac.New(sha1.New, []byte(key))
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

func main() {
	var (
		//flRoot               = flag.String("g", "/tmp/echoserver", "Path to use as the root of the docker runtime")
		//flDebug		= flag.Bool("D", false, "Enable debug mode")
		//flTest		= flag.Bool("t", false, "Enable test mode, no rabbitmq")
		flConfig = flag.String("c", "./conf/conf.json", "Config file")
	)
	log.Infof("pushd started...")
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

	logger, err := log.LoggerFromConfigAsFile("./conf/log.xml")
	if err != nil {
		fmt.Printf("Load log config failed: (%s)\n", err)
		os.Exit(1)
	}

	log.ReplaceLogger(logger)
	storage.NewInstance(&conf.Config)
	auth.NewInstance(&conf.Config)

	var ato uint32 = 60
	if conf.Config.AcceptTimeout > 0 {
		ato = conf.Config.AcceptTimeout
	}
	var rto uint32 = 60
	if conf.Config.ReadTimeout > 0 {
		rto = conf.Config.ReadTimeout
	}
	var wto uint32 = 60
	if conf.Config.WriteTimeout > 0 {
		wto = conf.Config.WriteTimeout
	}
	var hto uint32 = 200
	if conf.Config.HeartbeatTimeout > 0 {
		hto = conf.Config.HeartbeatTimeout
	}
	var mbl uint32 = 2048
	if conf.Config.MaxBodyLen > 0 {
		mbl = conf.Config.MaxBodyLen
	}
	var mc uint32 = 10000
	if conf.Config.MaxClients > 0 {
		mc = conf.Config.MaxClients
	}
	cometServer := comet.NewServer(ato, rto, wto, hto, mbl, mc)
	listener, err := cometServer.Init(conf.Config.Comet)
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

	StartRpcServer()

	log.Infof("pushd running")
	wg.Wait()
}
