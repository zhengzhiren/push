#!/bin/bash

USER=work
WORK_ROOT=/letv

THIS_DIR=$(dirname $(readlink -f $0) )

mkdir -p $WORK_ROOT/log/push/{pushd,pushapi}
chown -R $USER:$USER $WORK_ROOT/log/push/{pushd,pushapi}

mkdir -p $WORK_ROOT/run/push/{pushd,pushapi}
chown -R $USER:$USER $WORK_ROOT/run/push/{pushd,pushapi}

mkdir -p $WORK_ROOT/push
cp -r $THIS_DIR/pushd $WORK_ROOT/push/
ln -sf $WORK_ROOT/push/pushd/conf/conf_production.json $WORK_ROOT/push/pushd/conf/conf.json
cp -r $THIS_DIR/pushapi $WORK_ROOT/push/
ln -sf $WORK_ROOT/push/pushapi/conf/conf_production.json $WORK_ROOT/push/pushapi/conf/conf.json
chown -R $USER:$USER $WORK_ROOT/push
