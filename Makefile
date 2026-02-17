all: leto pkg/letopb cmd/leto/leto cmd/leto-cli/leto-cli check tools/artemis-frame-merger/artemis-frame-merger

generate:
	make -C cmd/leto generate

leto:
	make -C internal/leto
	make -C pkg/letopb

check:
	make -C internal/leto check
	make -C pkg/letopb check
	make -C cmd/leto check
	make -C cmd/leto-cli check

cmd/leto/leto:
	make -C cmd/leto

cmd/leto-cli/leto-cli:
	make -C cmd/leto-cli

clean:
	make -C cmd/leto clean
	make -C cmd/leto-cli clean

tools/artemis-frame-merger/artemis-frame-merger:
	make -C tools/artemis-frame-merger

.PHONY: clean leto check cmd/leto/leto cmd/leto-cli/leto-cli tools/artemis-frame-merger/artemis-frame-merger
