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
			log.Infof("get a event: %s", event.State.String())
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
		log.Errorf("zk.ChildrenW(\"%s\") error(%v)", path, err)
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
		log.Errorf("zk.Children(\"%s\") error(%v)", path, err)
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
	conn.Create(fpath, data, zk.FlagEphemeral, zk.WorldACL(zk.PermAll))
	/*if err != nil {
		return err
	}*/
	log.Infof("create zk node:%s", fpath)
	// watch self
	go func() {
		for {
			exist, _, watch, err := conn.ExistsW(fpath)
			if err != nil {
				log.Warnf("failed zk node \"%s\" set watch, [%v]", fpath, err)
				return
			}
			if !exist {
				log.Warnf("zk node \"%s\" not exist, [%v]", fpath, err)
				return
			}
			event := <-watch
			log.Infof("\"%s\" receive a event %v", fpath, event)
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

