#!/bin/bash

USER=work
WORK_ROOT=/letv

mkdir -p $WORK_ROOT/log/push/server
chown -R $USER:$USER $WORK_ROOT/log/push/server

mkdir -p $WORK_ROOT/run/push/server
chown -R $USER:$USER $WORK_ROOT/run/push/server

