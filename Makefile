all: leto leto/leto leto-cli/leto-cli check

leto: *.go letopb/*.go letopb/*.proto
	go generate
	go build



check:
	go test
	go vet
	make -C leto check
	make -C leto-cli check


leto/leto: *.go letopb/*.proto letopb/*.go
	make -C leto

leto-cli/leto-cli: *.go letopb/*.go letopb/*.proto
	make -C leto-cli

clean:
	make -C leto clean
	make -C leto-cli clean

.PHONY: clean leto  check
