//go:build linux

package leto

import "os/exec"

func (c *Config) determineFramegrabberType() error {
	cmd := exec.Command("modprobe", "hyperion2")
	if err := cmd.Run(); err != nil {
		c.FramegrabberType = HYPERION_FG
		return nil
	}
	cmd = exec.Command("modprobe", "egrabber")
	if err := cmd.Run(); err != nil {
		c.FramegrabberType = EURESYS_FG
		return nil
	}
	c.FramegrabberType = UNKNOWN_FG
	return nil
}
