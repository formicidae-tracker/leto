all: leto

leto: *.go ../../internal/leto/*.go ../../pkg/letopb/*.go ../../pkg/letopb/*.proto
	go generate
	go build

check: mock_main/artemis/artemis mock_main/ffmpeg/ffmpeg mock_main/coaxlink-firmware/coaxlink-firmware
	go test -coverprofile=cover.out -check.v
	go vet

artemis_help_v%.txt: FORCE
	docker run --rm ghcr.io/formicidae-tracker/artemis:$* artemis --help 2> $@

generate: artemis_help_v0.4.5.txt artemis_help_v0.5.0-rc1.txt


mock_main/artemis/artemis: mock_main/artemis/*.go
	make -C mock_main/artemis

mock_main/ffmpeg/ffmpeg: mock_main/ffmpeg/*.go
	make -C mock_main/ffmpeg

mock_main/coaxlink-firmware/coaxlink-firmware: mock_main/coaxlink-firmware/*.go
	make -C mock_main/coaxlink-firmware

clean:
	rm -f leto

.PHONY: clean check

FORCE:
