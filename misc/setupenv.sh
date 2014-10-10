#!/bin/bash

USER=work
WORK_ROOT=/letv

mkdir -p $WORK_ROOT/log/push/{pushd,pushapi}
chown -R $USER:$USER $WORK_ROOT/log/push/{pushd,pushapi}

mkdir -p $WORK_ROOT/run/push/{pushd,pushapi}
chown -R $USER:$USER $WORK_ROOT/run/push/{pushd,pushapi}

