#!/bin/bash

mkdir -p output
rm -rf output/*

cd server && go build -o ../output/pushd
[ $? -ne 0 ] && { echo "go build failed"; exit 1; }
cd - >/dev/null

cd pushtest && go build -o ../output/pushtest
[ $? -ne 0 ] && { echo "go build failed"; exit 1; }
cd - >/dev/null

cd api && go build -o ../output/pushapi
[ $? -ne 0 ] && { echo "go build failed"; exit 1; }
cd - >/dev/null

cp misc/* output/
cp -aR etc output/

