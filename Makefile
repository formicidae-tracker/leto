all: leto leto/leto leto-cli/leto-cli

leto: *.go letopb/*.go letopb/*.proto
	go generate
	go build
	go test
	go vet

check-main:
	go test

leto/leto: leto
	make -C leto

leto-cli/leto-cli: leto
	make -C leto-cli

clean:
	make -C leto clean
	make -C leto-cli clean

.PHONY: clean leto
