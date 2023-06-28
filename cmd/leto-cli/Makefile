all: leto-cli check

leto-cli: *.go ../../internal/leto/*.go ../../pkg/letopb/*.go ../../pkg/letopb/*.proto
	go generate
	go build

check:
	go test
	go vet

clean:
	rm -f leto-cli

.PHONY: clean check
