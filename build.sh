#!/bin/bash

go build
[ $? -ne 0 ] && { echo "go build failed"; exit 1; }
cd test && go build
[ $? -ne 0 ] && { echo "go build failed"; exit 1; }
cd -

mkdir -p output
rm -rf output/*
cp push output/
cp test/test output/
cp -aR etc output/

