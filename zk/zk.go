package zk

import (
	log "github.com/cihub/seelog"
	"errors"
	"github.com/samuel/go-zookeeper/zk"
	"github.com/chenyf/push/conf"
	"path"
	"strings"
	"encoding/json"
	"time"
)

var (
	ErrNoChild      = errors.New("zk: children is nil")
	ErrNodeNotExist = errors.New("zk: node not exist")
)

func Connect(addrs string, timeout time.Duration) (*zk.Conn, error) {
	addr := strings.Split(addrs, ",")
	conn, session, err := zk.Connect(addr, timeout)
	if err != nil {
		return nil, err
	}
	go func() {
		for {
			event := <-session
			log.Debug("get a event: %s", event.State.String())
		}
	}()
	return conn, nil
}

func GetNodesW(conn *zk.Conn, path string) ([]string, <-chan zk.Event, error) {
	nodes, stat, watch, err := conn.ChildrenW(path)
	if err != nil {
		if err == zk.ErrNoNode {
			return nil, nil, ErrNodeNotExist
		}
		log.Error("zk.ChildrenW(\"%s\") error(%v)", path, err)
		return nil, nil, err
	}
	if stat == nil {
		return nil, nil, ErrNodeNotExist
	}
	if len(nodes) == 0 {
		return nil, nil, ErrNoChild
	}
	return nodes, watch, nil
}

func GetNodes(conn *zk.Conn, path string) ([]string, error) {
	nodes, stat, err := conn.Children(path)
	if err != nil {
		if err == zk.ErrNoNode {
			return nil, ErrNodeNotExist
		}
		log.Error("zk.Children(\"%s\") error(%v)", path, err)
		return nil, err
	}
	if stat == nil {
		return nil, ErrNodeNotExist
	}
	if len(nodes) == 0 {
		return nil, ErrNoChild
	}
	return nodes, nil
}

func Register(conn *zk.Conn, fpath string, data []byte) error {
	tpath, err := conn.Create(fpath, data, zk.FlagEphemeral, zk.WorldACL(zk.PermAll))
	if err != nil {
		return err
	}
	log.Debug("create zk node:%s", tpath)
	// watch self
	go func() {
		for {
			exist, _, watch, err := conn.ExistsW(tpath)
			if err != nil {
				log.Warn("failed zk node \"%s\" set watch, [%v]", tpath, err)
				return
			}
			if !exist {
				log.Warn("zk node \"%s\" not exist, [%v]", tpath, err)
				return
			}
			event := <-watch
			log.Info("\"%s\" receive a event %v", tpath, event)
		}
	}()
	return nil
}

func InitZk() error {
	conn, err := Connect(conf.Config.ZooKeeper.Addr, conf.Config.ZooKeeper.Timeout*time.Second)
	if err != nil {
		return err
	}
	conn.Create(conf.Config.ZooKeeper.Path, []byte(""), 0,  zk.WorldACL(zk.PermAll))
	fpath := path.Join(conf.Config.ZooKeeper.Path, conf.Config.ZooKeeper.Node)
	data, _ := json.Marshal(conf.Config.ZooKeeper.NodeInfo)
	err = Register(conn, fpath, data)
	if err != nil {
		return err
	}
	return nil
}
