package main

import (
	. "gopkg.in/check.v1"
)

type ArtemisManagerSuite struct{}

var _ = Suite(&ArtemisManagerSuite{})

func (s *ArtemisManagerSuite) TestCanExtractVariantFromUpdateToolOutput(c *C) {
	txt := `1 Coaxlink card available:

0 - PC1635 - Coaxlink Quad G3 DF (1-df-camera)
    ------------------------------------------
    Product code:      PC1635
    Serial number:     KDF00296
    Part number:       00005444-12
    Firmware variant:  33 (1-df-camera)
    Firmware revision: 273
    Firmware checksum: cbd5798e776e488976734b4b0808a95df2c31f6f
    Firmware status:   OK

    To install firmware variant "1-camera" revision 273, run 'coaxlink-firmware install "1-camera" --card=0'

    To install firmware variant "1-camera, line-scan" revision 273, run 'coaxlink-firmware install "1-camera, line-scan" --card=0'

    To install firmware variant "1-df-camera, line-scan" revision 273, run 'coaxlink-firmware install "1-df-camera, line-scan" --card=0'
`
	res, err := extractCoaxlinkFirmwareOutput([]byte(txt))
	c.Assert(err, IsNil)
	c.Check(res, Equals, "1-df-camera")
	res, err = extractCoaxlinkFirmwareOutput([]byte(""))
	c.Check(err, ErrorMatches, `Could not determine firmware variant in output: ''`)
	c.Check(res, Equals, "")
}

func (s *ArtemisManagerSuite) TestCheckFirmwareVariant(c *C) {
	testdata := []struct {
		C        NodeConfiguration
		Variant  string
		Expected string
	}{
		{
			Expected: `unexpected firmware variant  \(expected: 1-camera\)`,
		},
		{
			Variant:  "1-df-camera",
			Expected: `unexpected firmware variant 1-df-camera \(expected: 1-camera\)`,
		},
		{
			Variant: "1-camera",
		},
		{
			C:        NodeConfiguration{Master: "foo"},
			Variant:  "",
			Expected: `unexpected firmware variant  \(expected: 1-df-camera\)`,
		},
		{
			C:        NodeConfiguration{Master: "foo"},
			Variant:  "1-camera",
			Expected: `unexpected firmware variant 1-camera \(expected: 1-df-camera\)`,
		},
	}

	for _, d := range testdata {
		err := checkFirmwareVariant(d.C, d.Variant)
		if len(d.Expected) == 0 {
			c.Check(err, IsNil)
			continue
		}
		c.Check(err, ErrorMatches, d.Expected)
	}
}
