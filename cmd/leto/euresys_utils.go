package main

import (
	"fmt"
	"os/exec"
	"regexp"
)

var coaxlinkFirmwareCommandName = "coaxlink-firmware"

func getAndCheckFirmwareVariant(c NodeConfiguration) error {
	variant, err := getFirmwareVariant()
	if err != nil {
		return err
	}
	return checkFirmwareVariant(c, variant)
}

func getFirmwareVariant() (string, error) {
	cmd := exec.Command(coaxlinkFirmwareCommandName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("could not check firmware variant: %s", err)
	}

	return extractCoaxlinkFirmwareOutput(output)
}

func extractCoaxlinkFirmwareOutput(output []byte) (string, error) {
	rx := regexp.MustCompile(`Firmware variant:\W+[0-9]+\W+\(([0-9a-z\-]+)\)`)
	m := rx.FindStringSubmatch(string(output))
	if len(m) == 0 {
		return "", fmt.Errorf("Could not determine firmware variant in output: '%s'", output)
	}
	return m[1], nil
}

func checkFirmwareVariant(c NodeConfiguration, variant string) error {
	expected := "1-camera"
	if c.IsMaster() == false {
		expected = "1-df-camera"
	}

	if variant != expected {
		return fmt.Errorf("unexpected firmware variant %s (expected: %s)", variant, expected)
	}

	return nil
}
