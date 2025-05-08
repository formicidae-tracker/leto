//go:build !linux

package leto

import "fmt"

func (c *Config) determineFramegrabberType() error {
	return fmt.Errorf("This is only implemented on linux")
}
