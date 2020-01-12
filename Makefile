build-proto:
	protoc --go_out=plugins=grpc:helloworld -I helloworld helloworld/helloworld.proto

build-server:
	go build -o bin/server ./server

build-client:
	go build -o bin/client ./client

build-proxy:
	go build -o bin/proxy ./proxy

build-stub:
	go build -o bin/stub ./stub

start-server:
	pkill -f bin/server | true
	bin/server &
