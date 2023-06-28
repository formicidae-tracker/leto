all: leto pkg/letopb cmd/leto/leto cmd/leto-cli/leto-cli check

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

.PHONY: clean leto check cmd/leto/leto cmd/leto-cli/leto-cli
