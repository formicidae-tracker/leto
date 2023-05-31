all: leto-cli check

leto-cli: *.go ../*.go ../letopb/*.go ../letopb/*.proto
	go generate
	go build

check:
	go test -check.v
	go vet

clean:
	rm -f leto-cli

.PHONY: clean check
