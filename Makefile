build-proto:
	protoc --go_out=plugins=grpc:proto -I proto proto/helloworld.proto
