all: clean init pushd pushapi pushtest notifyapi tarball

init:
	mkdir -p output
	rm -rf output/*
	mkdir -p output/{pushd,pushapi,pushtest,notifyapi}

pushd: init
	cd pushd && go build -o ../output/pushd/pushd 

pushapi: init
	cd pushapi && go build -o ../output/pushapi/pushapi

pushtest: init
	cd pushtest && go build -o ../output/pushtest/pushtest

notifyapi: init
	cd notifyapi && go build -o ../output/notifyapi/notifyapi

tarball: init pushd pushapi pushtest notifyapi
	cp misc/* output/
	cp -aR pushd/conf output/pushd/
	cp -aR pushd/control.sh output/pushd/
	cp -aR pushapi/conf output/pushapi/
	cp -aR pushapi/control.sh output/pushapi/
	cp -aR notifyapi/conf output/notifyapi/
	cp -aR notifyapi/control.sh output/notifyapi/
	tar -czf push.tgz output

clean:
	rm -rf output push.tgz

TEST_DIRS:=auth storage devcenter pushapi utils

test:
	@for dir in $(TEST_DIRS); do \
		cd $(CURDIR)/$$dir && go test; \
	done

bench:
	@for dir in $(TEST_DIRS); do \
		cd $(CURDIR)/$$dir && go test -bench .; \
	done
