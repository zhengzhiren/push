package zk

import (
	log "github.com/cihub/seelog"
	"errors"
	"github.com/gooo000/go-zookeeper/zk"
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
	_, err := conn.Create(fpath, data, zk.FlagEphemeral, zk.WorldACL(zk.PermAll))
	if err != nil && err != zk.ErrNodeExists {
		return err
	}
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
	_, err = conn.Create(conf.Config.ZooKeeper.Path, []byte(""), 0,  zk.WorldACL(zk.PermAll))
	if err != nil && err != zk.ErrNodeExists {
		return err
	}
	fpath := path.Join(conf.Config.ZooKeeper.Path, conf.Config.ZooKeeper.Node)
	data, _ := json.Marshal(conf.Config.ZooKeeper.NodeInfo)
	err = Register(conn, fpath, data)
	if err != nil {
		return err
	}
	return nil
}

