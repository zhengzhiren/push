#!/bin/bash

mkdir -p output
rm -rf output/*
mkdir -p output/{pushd,pushapi,pushtest}

cd pushd && go build -o ../output/pushd/pushd
[ $? -ne 0 ] && { echo "build 'pushd' failed"; exit 1; }
cd - >/dev/null

cd pushapi && go build -o ../output/pushapi/pushapi
[ $? -ne 0 ] && { echo "build 'pushapi' failed"; exit 1; }
cd - >/dev/null

cd pushtest && go build -o ../output/pushtest/pushtest
[ $? -ne 0 ] && { echo "build 'pushtest' failed"; exit 1; }
cd - >/dev/null

cp misc/* output/
cp -aR pushd/conf output/pushd/
cp -aR pushd/control.sh output/pushd/

cp -aR pushapi/conf output/pushapi/
cp -aR pushapi/control.sh output/pushapi/

