#!/bin/bash

USER=work
WORK_ROOT=/letv

THIS_DIR=$(dirname $(readlink -f $0) )

mkdir -p $WORK_ROOT/logs/push/{pushd,pushapi,notifyapi}
chown -R $USER:$USER $WORK_ROOT/logs/push/{pushd,pushapi,notifyapi}

mkdir -p $WORK_ROOT/run/push/{pushd,pushapi,notifyapi}
chown -R $USER:$USER $WORK_ROOT/run/push/{pushd,pushapi,notifyapi}

mkdir -p $WORK_ROOT/push
cp -r $THIS_DIR/pushd $WORK_ROOT/push/
ln -sf $WORK_ROOT/push/pushd/conf/conf_production.json $WORK_ROOT/push/pushd/conf/conf.json
cp -r $THIS_DIR/pushapi $WORK_ROOT/push/
ln -sf $WORK_ROOT/push/pushapi/conf/conf_production.json $WORK_ROOT/push/pushapi/conf/conf.json
cp -r $THIS_DIR/notifyapi $WORK_ROOT/push/
ln -sf $WORK_ROOT/push/notifyapi/conf/conf_production.json $WORK_ROOT/push/notifyapi/conf/conf.json
chown -R $USER:$USER $WORK_ROOT/push
