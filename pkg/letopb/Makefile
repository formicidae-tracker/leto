all: leto_service.pb.go leto_service_grpc.pb.go

%.go: leto_service.proto
	go generate

check:
	go test -coverprofile=cover.out

.PHONY: check
