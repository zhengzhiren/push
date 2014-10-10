#!/bin/bash

mkdir -p output
rm -rf output/*

cd pushd && go build -o ../output/pushd
[ $? -ne 0 ] && { echo "build 'pushd' failed"; exit 1; }
cd - >/dev/null

cd pushapi && go build -o ../output/pushapi
[ $? -ne 0 ] && { echo "build 'pushapi' failed"; exit 1; }
cd - >/dev/null

cd pushtest && go build -o ../output/pushtest
[ $? -ne 0 ] && { echo "build 'pushtest' failed"; exit 1; }
cd - >/dev/null

cp misc/* output/
cp -aR etc output/

