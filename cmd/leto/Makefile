all: leto

leto: *.go ../../internal/leto/*.go ../../pkg/letopb/*.go ../../pkg/letopb/*.proto
	go generate
	go build

check: mock_main/artemis/artemis mock_main/ffmpeg/ffmpeg mock_main/coaxlink-firmware/coaxlink-firmware
	go test -coverprofile=cover.out -check.v
	go vet

mock_main/artemis/artemis: mock_main/artemis/*.go
	make -C mock_main/artemis

mock_main/ffmpeg/ffmpeg: mock_main/ffmpeg/*.go
	make -C mock_main/ffmpeg

mock_main/coaxlink-firmware/coaxlink-firmware: mock_main/coaxlink-firmware/*.go
	make -C mock_main/coaxlink-firmware

clean:
	rm -f leto

.PHONY: clean check
