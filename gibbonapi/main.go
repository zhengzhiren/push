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
	"github.com/chenyf/push/conf"
	"github.com/chenyf/push/mq_rpc"
)

var (
	exchange string = "gibbon_rpc_exchange"
)

func main() {

	var (
		logConfigFile = flag.String("l", "./etc/log.xml", "Log config file")
		configFile    = flag.String("c", "./etc/conf.json", "Config file")
	)

	flag.Parse()

	err := conf.LoadConfig(*configFile)
	if err != nil {
		fmt.Printf("LoadConfig (%s) failed: (%s)\n", *configFile, err)
		os.Exit(1)
	}

	logger, err := log.LoggerFromConfigAsFile(*logConfigFile)
	if err != nil {
		fmt.Printf("Load log config failed: (%s)\n", err)
		os.Exit(1)
	}

	log.ReplaceLogger(logger)

	waitGroup := &sync.WaitGroup{}

	waitGroup.Add(1)
	rpcClient, err = mq_rpc.NewRpcClient(conf.Config.Rabbit.Uri, exchange)
	if err != nil {
		log.Criticalf("Create RPC client failed: %s", err)
		os.Exit(1)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

	go func() {
		sig := <-c
		log.Infof("Received signal '%v', exiting\n", sig)
		log.Infof("leave 1")
		rpcClient.Close()
		waitGroup.Done()
		log.Infof("leave 2")
	}()

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
		&rest.Route{"GET", "/status", getStatus},
	)
	if err != nil {
		log.Criticalf("http SetRoutes: ", err)
		os.Exit(1)
	}

	// new API
	http.Handle("/api/v1/", http.StripPrefix("/api/v1", &handler))

	err = http.ListenAndServe(addr, nil)
	if err != nil {
		log.Criticalf("http listen: ", err)
		os.Exit(1)
	}
}
