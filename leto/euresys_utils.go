package main

import (
	"fmt"
	"os/exec"
	"regexp"

	"github.com/blang/semver"
)

func getAndCheckFirmwareVariant(c NodeConfiguration) error {
	variant, err := getFirmwareVariant()
	if err != nil {
		return err
	}
	return checkFirmwareVariant(c, variant)
}

func getFirmwareVariant() (string, error) {
	cmd := exec.Command("coaxlink-firmware")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("Could not check slave firmware variant")
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

func checkArtemisVersion(actual, minimal string) error {
	a, err := semver.ParseTolerant(actual)
	if err != nil {
		return err
	}
	m, err := semver.ParseTolerant(minimal)
	if err != nil {
		return err
	}

	if m.Major == 0 {
		if a.Major != 0 || a.Minor != m.Minor {
			return fmt.Errorf("Unexpected major version v%d.%d (expected: v%d.%d)", a.Major, a.Minor, m.Major, m.Minor)
		}
	} else if m.Major != a.Major {
		return fmt.Errorf("Unexpected major version v%d (expected: v%d)", a.Major, m.Major)
	}

	if a.GE(m) == false {
		return fmt.Errorf("Invalid version v%s (minimal: v%s)", a, m)
	}

	return nil
}
