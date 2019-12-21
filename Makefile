build-proto:
	protoc --go_out=plugins=grpc:helloworld -I helloworld helloworld/helloworld.proto
