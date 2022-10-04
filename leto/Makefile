all: leto check

leto: *.go ../*.go ../letopb/*.go ../letopb/*.proto
	go generate
	go build

check:
	go test
	go vet

clean:
	rm -f leto

.PHONY: clean check
