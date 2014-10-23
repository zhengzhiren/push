package main

import (
	"flag"
	"os"
	"time"
	"sync"
	"strings"
	"os/signal"
	"syscall"
	"net/http"
	"fmt"
	"crypto/hmac"
	"crypto/sha1"
	"github.com/chenyf/push/conf"
	"github.com/chenyf/push/auth"
	"github.com/chenyf/push/mq"
	"github.com/chenyf/push/comet"
	log "github.com/cihub/seelog"
	"github.com/chenyf/push/zk"
)

type CommandRequest struct {
	Uid		string	`json:"uid"`
	Cmd		string	`json:"cmd"`
}

type CommandResponse struct {
	Status	int		`json:"status"`
	Error	string	`json:"error"`
}

func getStatus(w http.ResponseWriter, r *http.Request) {
	size := comet.DevicesMap.Size()
	fmt.Fprintf(w, "total register device: %d\n", size)
}

func checkAuthz(uid string, devid string) bool {
	return false
}

func sign(path string, query map[string]string) []byte{
	uid := query["uid"]
	rid := query["rid"]
	tid := query["tid"]
	src := query["src"]
	tm := query["tm"]
	pmtt := query["pmtt"]

	raw := []string{path, uid, rid, tid, src, tm, pmtt}
	args := []string{}
	x := []int{6, 5, 4, 3, 2, 1, 0}
	for _, item := range(x) {
		args = append(args, raw[item])
	}
	data := strings.Join(args, "")
	key := "xnRzFxoCDRVRU2mNQ7AoZ5MCxpAR7ntnmlgRGYav"
	mac := hmac.New(sha1.New, []byte(key))
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

type InputMsg struct {
	AppId		string	`json:"app_id"`
	PushType	int		`json:"push_type"`
	RegId		string	`json:"reg_id"`
	MsgType		int		`json:"msg_type"`
	TTL			uint64	`json:"ttl"`
	MsgId		int64	`json:"msg_id"`
	Payload		string	`json:"payload"`
}

func main() {
	var (
		//flRoot               = flag.String("g", "/tmp/echoserver", "Path to use as the root of the docker runtime")
		//flDebug		= flag.Bool("D", false, "Enable debug mode")
		//flTest		= flag.Bool("t", false, "Enable test mode, no rabbitmq")
		flConfig	= flag.String("c", "./conf/conf.json", "Config file")
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

	logger, err := log.LoggerFromConfigAsFile("./conf/log.xml")
	if err != nil {
		fmt.Printf("Load log config failed: (%s)\n", err)
		os.Exit(1)
	}

	log.ReplaceLogger(logger)

	waitGroup := &sync.WaitGroup{}
	var mqConsumer *mq.Consumer = nil
	cometServer := comet.NewServer()
	if conf.Config.AcceptTimeout > 0 {
		cometServer.SetAcceptTimeout(time.Duration(conf.Config.AcceptTimeout))
	}
	if conf.Config.ReadTimeout > 0 {
		cometServer.SetReadTimeout(time.Duration(conf.Config.ReadTimeout))
	}
	if conf.Config.WriteTimeout > 0 {
		cometServer.SetWriteTimeout(time.Duration(conf.Config.WriteTimeout))
	}
	if conf.Config.HeartbeatTimeout > 0 {
		cometServer.SetHeartbeatTimeout(time.Duration(conf.Config.HeartbeatTimeout))
	}

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

	listener, err := cometServer.Init(conf.Config.Comet)
	if err != nil {
		log.Critical(err)
		os.Exit(1)
	}

	if conf.Config.ZooKeeper.Enable {
		err = zk.InitZk(); if err != nil {
			log.Critical(err)
			os.Exit(1)
		}
	}
	auth.NewInstance(conf.Config.Auth.Provider)

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
		waitGroup.Done()
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
	waitGroup.Add(1)
	go func() {
		http.HandleFunc("/status", getStatus)
		err := http.ListenAndServe(conf.Config.Web, nil)
		if err != nil {
			log.Critical("http listen: ", err)
			os.Exit(1)
		}
	}()
	waitGroup.Wait()
}

