package zk

import (
	log "github.com/cihub/seelog"
	"github.com/gooo000/go-zookeeper/zk"
	"time"
	"path"
	"encoding/json"
	"math/rand"
)

const (
    // node event
    eventNodeAdd    = 1
    eventNodeDel    = 2
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
		_, _, watch, err := conn.ChildrenW(fpath)
		if err != nil {
			log.Infof("watch children with err [%v]", err)
		}
		select {
			case event := <-watch:
				log.Infof("zk path: \"%s\" receive a event %v", fpath, event)
				if event.Type == zk.EventNodeChildrenChanged {
					nodes, _, err := conn.Children(fpath)
					if err != nil {
						log.Infof("get children with err [%v]", err)
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
				}
		}
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
			log.Infof("add node: \"%s\"", ev.Key)
			npath := path.Join(fpath, ev.Key)
			data, _, err := conn.Get(npath)
			if err != nil {
				log.Errorf("failed to get zk node \"%s\"", npath)
			}
			var addr string
			json.Unmarshal(data, &addr)
			tmpMap[ev.Key] = &CometNodeInfo{
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
		cometNodeInfoMap = tmpMap
		var tmpList []string
		for k, _ := range tmpMap {
			tmpList = append(tmpList, k)
		}
		cometNodeList = tmpList
		log.Infof("cometNodeInfofMap len: %d", len(cometNodeInfoMap))
	}
}

func initCometNodeInfo(conn *zk.Conn, fpath string, ch chan *CometNodeEvent) error{
	nodes, _, err := conn.Children(fpath)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		if _, ok := cometNodeInfoMap[node]; !ok {
			ch <- &CometNodeEvent{Event: eventNodeAdd, Key: node}
		}
	}
	return nil
}

func GetComet() *CometNodeInfo {
	if len(cometNodeInfoMap) == 0 {
		return nil
	}
	i := rand.Intn(len(cometNodeList))
	node := cometNodeList[i]
	return cometNodeInfoMap[node]
}

func InitWatcher(addrs string, timeout time.Duration, fpath string) error {
	conn, err := Connect(addrs, timeout)
	if err != nil {
		return err
	}
	_, err = conn.Create(fpath, []byte(""), 0, zk.WorldACL(zk.PermAll))
	if err != nil && err != zk.ErrNodeExists {
		return err
	}

	ch := make(chan *CometNodeEvent, 1024)
	if err := initCometNodeInfo(conn, fpath, ch); err != nil {
		log.Infof("failed to init cometNodeInfo with err [%v]", err)
	}
	go handleCometNodeEvent(conn, fpath, ch)
	go watchCometRoot(conn, fpath, ch)
	return nil
}

