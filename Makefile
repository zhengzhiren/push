all: clean init pushd pushapi pushtest gibbonapi tarball

init:
	mkdir -p output
	rm -rf output/*
	mkdir -p output/{pushd,pushapi,gibbonapi,pushtest}

pushd: init
	cd pushd && go build -o ../output/pushd/pushd 

pushapi: init
	cd pushapi && go build -o ../output/pushapi/pushapi

pushtest: init
	cd pushtest && go build -o ../output/pushtest/pushtest

gibbonapi: init
	cd gibbonapi && go build -o ../output/gibbonapi/gibbonapi

tarball: init pushd pushapi pushtest
	cp misc/* output/
	cp -aR pushd/conf output/pushd/
	cp -aR pushd/control.sh output/pushd/
	cp -aR pushapi/conf output/pushapi/
	cp -aR pushapi/control.sh output/pushapi/
	cp -aR gibbonapi/etc output/gibbonapi/
	cp -aR gibbonapi/control.sh output/gibbonapi/
	tar -czf push.tgz output

clean:
	rm -rf output push.tgz

