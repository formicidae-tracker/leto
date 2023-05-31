all: leto leto/leto leto-cli/leto-cli check

leto: *.go letopb/*.go letopb/*.proto
	go generate
	go build

check:
	go test -check.v
	go vet
	make -C leto check
	make -C leto-cli check

leto/leto: *.go letopb/*.proto letopb/*.go leto/*.go
	make -C leto leto

leto-cli/leto-cli: *.go letopb/*.go letopb/*.proto leto-cli/*.go
	make -C leto-cli leto-cli

clean:
	make -C leto clean
	make -C leto-cli clean

.PHONY: clean leto  check
