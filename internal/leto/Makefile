all: version.go

version.go:
	go generate

check:
	go test -coverprofile=cover.out
	go vet

.PHONY: version.go check
