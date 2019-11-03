# Makefile

export GOPATH := /Users/abowen/Downloads/go_dev

local: 
	go run *.go -c conf.txt
	cat de.pem

local-debug:
	go run *.go -vvv -c conf.txt
	cat de.pem

build:
	echo $$GOPATH
	go get -d
	rm ./bin/linux64/*
	env GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=v0.3" -v -o ./bin/linux64/self_enroll
	tar -czvf bin/linux64/self_enroll_linux_x64_v0.3.tgz -C bin/linux64 self_enroll

deploy: build
	rsync -arv conf.txt centos@proddb:.
	rsync -arv ./bin/linux64/self_enroll centos@proddb:.
	ssh centos@proddb "chmod +x self_enroll && sudo ./self_enroll -c conf.txt"
	