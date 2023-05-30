all: leto check

leto: *.go ../*.go ../letopb/*.go ../letopb/*.proto
	go generate
	go build

check:
	go test -coverprofile=cover.out -check.v1
	go vet

clean:
	rm -f leto

.PHONY: clean check
