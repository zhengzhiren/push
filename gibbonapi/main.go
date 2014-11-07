package main

import (
	"flag"
	"fmt"
	log "github.com/cihub/seelog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/ant0ine/go-json-rest/rest"
	"github.com/chenyf/gibbon/conf"
	//"github.com/chenyf/gibbon/zk"
)

func main() {

	var (
		flConfig = flag.String("c", "./etc/conf.json", "Config file")
	)

	flag.Parse()

	err := conf.LoadConfig(*flConfig)
	if err != nil {
		fmt.Printf("LoadConfig (%s) failed: (%s)\n", *flConfig, err)
		os.Exit(1)
	}

	logger, err := log.LoggerFromConfigAsFile("./etc/log.xml")
	if err != nil {
		fmt.Printf("Load log config failed: (%s)\n", err)
		os.Exit(1)
	}

	log.ReplaceLogger(logger)

	waitGroup := &sync.WaitGroup{}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	waitGroup.Add(1)
	go func() {
		sig := <-c
		log.Infof("Received signal '%v', exiting\n", sig)
		log.Infof("leave 1")
		waitGroup.Done()
		log.Infof("leave 2")
	}()

	//if conf.Config.ZooKeeper.Enable {
	//	if err := zk.ProduceZnode(conf.Config.ZooKeeper.Addr,
	//		conf.Config.ZooKeeper.Root,
	//		conf.Config.ZooKeeper.CometAddr,
	//		time.Duration(conf.Config.ZooKeeper.Timeout)*time.Second); err != nil {
	//		log.Criticalf("ProduceZnode failed: %s", err.Error())
	//		os.Exit(1)
	//	}
	//	if err := zk.Watch(conf.Config.ZooKeeper.Addr,
	//		conf.Config.ZooKeeper.Root,
	//		time.Duration(conf.Config.ZooKeeper.Timeout)*time.Second); err != nil {
	//		log.Criticalf("Watch failed: %s", err.Error())
	//		os.Exit(1)
	//	}
	//}

	go startHttp(conf.Config.Web, conf.Config.CommandTimeout)

	waitGroup.Wait()
}

func startHttp(addr string, cmdTimeout int) {
	log.Infof("Starting HTTP server on %s, command timeout: %ds", addr, cmdTimeout)
	commandTimeout = cmdTimeout

	handler := rest.ResourceHandler{}
	err := handler.SetRoutes(
		&rest.Route{"GET", "/devices", getDeviceList},
		&rest.Route{"GET", "/devices/:devid", getDevice},
		&rest.Route{"POST", "/devices/:devid", controlDevice},
		&rest.Route{"GET", "/servers", getGibbon},
		&rest.Route{"GET", "/status", getStatus},
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

	// the adapter API for old system
	http.HandleFunc("/router/command", postRouterCommand)
	http.HandleFunc("/router/list", getRouterList)

	// new API
	http.Handle("/api/v1/", http.StripPrefix("/api/v1", &handler))

	err = http.ListenAndServe(addr, nil)
	if err != nil {
		log.Criticalf("http listen: ", err)
		os.Exit(1)
	}
}
