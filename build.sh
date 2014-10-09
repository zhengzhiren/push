#!/bin/bash

go build -o pushd
[ $? -ne 0 ] && { echo "go build failed"; exit 1; }
cd test && go build -o pushtest
[ $? -ne 0 ] && { echo "go build failed"; exit 1; }
cd -

mkdir -p output
rm -rf output/*
cp pushd output/
cp setupenv.sh output/
cp test/pushtest output/
cp -aR etc output/
cp -aR bin output/

