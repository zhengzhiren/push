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
	cometNodeInfoMap = make(map[string]*CometNodeInfo)
	cometNodeList []string
)

// CometNodeData stored in zookeeper
type CometNodeInfo struct {
	TcpAddr string `json:"tcp"`
	Weight  int    `json:"weight"`
}

type CometNodeEvent struct {
	// node name(node1, node2...)
	Key string
	// node info
	Value *CometNodeInfo
	// event type
	Event int
}

// watchCometRoot watch the gopush root node for detecting the node add/del.
func watchCometRoot(conn *zk.Conn, fpath string, ch chan *CometNodeEvent) error {
	for {
		nodes, watch, err := GetNodesW(conn, fpath)
		if err == ErrNodeNotExist {
			log.Warn("zk don't have node \"%s\"", fpath)
			time.Sleep(waitNodeDelaySecond)
			continue
		} else if err == ErrNoChild {
			log.Warn("zk don't have any children in \"%s\"", fpath)
			for node, _ := range cometNodeInfoMap {
				ch <- &CometNodeEvent{Event: eventNodeDel, Key: node}
			}
			time.Sleep(waitNodeDelaySecond)
			continue
		} else if err != nil {
			log.Error("getNodes error(%v)", err)
			time.Sleep(waitNodeDelaySecond)
			continue
		}
		nodesMap := map[string]bool{}
		// handle new add nodes
		for _, node := range nodes {
			if _, ok := cometNodeInfoMap[node]; !ok {
				ch <- &CometNodeEvent{Event: eventNodeAdd, Key: node}
			}
			nodesMap[node] = true
		}
		// handle delete nodes
		for node, _ := range cometNodeInfoMap {
			if _, ok := nodesMap[node]; !ok {
				ch <- &CometNodeEvent{Event: eventNodeDel, Key: node}
			}
		}
		event := <-watch
		log.Info("zk path: \"%s\" receive a event %v", fpath, event)
	}
}

func handleCometNodeEvent(conn *zk.Conn, fpath string, ch chan *CometNodeEvent) {
	for {
		ev := <-ch
		tmpMap := make(map[string]*CometNodeInfo, len(cometNodeInfoMap))
		for k, v := range cometNodeInfoMap {
			tmpMap[k] = v
		}
		if ev.Event == eventNodeAdd {
			log.Info("add node: \"%s\"", ev.Key)
			npath := path.Join(fpath, ev.Key)
			data, _, err := conn.Get(npath)
			if err != nil {
				log.Error("failed to get zk node \"%s\"", npath)
			}
			var addr string
			json.Unmarshal(data, &addr)
			tmpMap[ev.Key] = &CometNodeInfo{
				TcpAddr: addr,
				Weight: 1,
			}
		} else if ev.Event == eventNodeDel {
			log.Info("del node: \"%s\"", ev.Key)
			delete(tmpMap, ev.Key)
		} else {
			log.Error("unknown node event: %d", ev.Event)
			panic("unknown node event")
		}
		cometNodeInfoMap = tmpMap
		var tmpList []string
		for k, _ := range tmpMap {
			tmpList = append(tmpList, k)
		}
		cometNodeList = tmpList
		log.Info("cometNodeInfoMap len: %d", len(cometNodeInfoMap))
	}
}

func GetComet() *CometNodeInfo {
	if len(cometNodeInfoMap) == 0 {
		return nil
	}
	i := rand.Intn(len(cometNodeList))
	node := cometNodeList[i]
	return cometNodeInfoMap[node]
}

func InitZK(addrs string, timeout time.Duration, fpath string) error {
	conn, err := Connect(addrs, timeout)
	if err != nil {
		log.Warn("failed to connect zk")
		return err
	}
	ch := make(chan *CometNodeEvent, 1024)
	go handleCometNodeEvent(conn, fpath, ch)
	go watchCometRoot(conn, fpath, ch)
	return nil
}

