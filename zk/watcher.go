package zk

import (
	log "github.com/cihub/seelog"
	"github.com/samuel/go-zookeeper/zk"
	"time"
	"path"
	"encoding/json"
	"math/rand"
)

const (
    // node event
    eventNodeAdd    = 1
    eventNodeDel    = 2

    // wait node
    waitNodeDelay       = 3
    waitNodeDelaySecond = waitNodeDelay * time.Second
)

var (
	cometNodeInfofMap = make(map[string]*CometNodeInfof)
	cometNodeList []string
)

// CometNodeData stored in zookeeper
type CometNodeInfof struct {
	TcpAddr string `json:"tcp"`
	Weight  int    `json:"weight"`
}

type CometNodeEvent struct {
	// node name(node1, node2...)
	Key string
	// node info
	Value *CometNodeInfof
	// event type
	Event int
}

// watchCometRoot watch the gopush root node for detecting the node add/del.
func watchCometRoot(conn *zk.Conn, fpath string, ch chan *CometNodeEvent) error {
	for {
		nodes, watch, err := GetNodesW(conn, fpath)
		if err == ErrNodeNotExist {
			log.Warnf("zk don't have node \"%s\"", fpath)
			time.Sleep(waitNodeDelaySecond)
			continue
		} else if err == ErrNoChild {
			log.Warnf("zk don't have any children in \"%s\"", fpath)
			for node, _ := range cometNodeInfofMap {
				ch <- &CometNodeEvent{Event: eventNodeDel, Key: node}
			}
			time.Sleep(waitNodeDelaySecond)
			continue
		} else if err != nil {
			log.Errorf("getNodes error(%v)", err)
			time.Sleep(waitNodeDelaySecond)
			continue
		}
		nodesMap := map[string]bool{}
		// handle new add nodes
		for _, node := range nodes {
			if _, ok := cometNodeInfofMap[node]; !ok {
				ch <- &CometNodeEvent{Event: eventNodeAdd, Key: node}
			}
			nodesMap[node] = true
		}
		// handle delete nodes
		for node, _ := range cometNodeInfofMap {
			if _, ok := nodesMap[node]; !ok {
				ch <- &CometNodeEvent{Event: eventNodeDel, Key: node}
			}
		}
		event := <-watch
		log.Infof("zk path: \"%s\" receive a event %v", fpath, event)
	}
}

func handleCometNodeEvent(conn *zk.Conn, fpath string, ch chan *CometNodeEvent) {
	for {
		ev := <-ch
		tmpMap := make(map[string]*CometNodeInfof, len(cometNodeInfofMap))
		for k, v := range cometNodeInfofMap {
			tmpMap[k] = v
		}
		if ev.Event == eventNodeAdd {
			log.Infof("add node: \"%s\"", ev.Key)
			npath := path.Join(fpath, ev.Key)
			data, _, err := conn.Get(npath)
			if err != nil {
				log.Errorf("failed to get zk node \"%s\"", npath)
			}
			var addr string
			json.Unmarshal(data, &addr)
			tmpMap[ev.Key] = &CometNodeInfof{
				TcpAddr: addr,
				Weight: 1,
			}
		} else if ev.Event == eventNodeDel {
			log.Infof("del node: \"%s\"", ev.Key)
			delete(tmpMap, ev.Key)
		} else {
			log.Errorf("unknown node event: %d", ev.Event)
			panic("unknown node event")
		}
		cometNodeInfofMap = tmpMap
		var tmpList []string
		for k, _ := range tmpMap {
			tmpList = append(tmpList, k)
		}
		cometNodeList = tmpList
		log.Infof("cometNodeInfofMap len: %d", len(cometNodeInfofMap))
	}
}

func GetComet() *CometNodeInfof {
	if len(cometNodeInfofMap) == 0 {
		return nil
	}
	i := rand.Intn(len(cometNodeList))
	node := cometNodeList[i]
	return cometNodeInfofMap[node]
}

func InitWatcher(addrs string, timeout time.Duration, fpath string) error {
	conn, err := Connect(addrs, timeout)
	if err != nil {
		return err
	}
	ch := make(chan *CometNodeEvent, 1024)
	go handleCometNodeEvent(conn, fpath, ch)
	go watchCometRoot(conn, fpath, ch)
	return nil
}

