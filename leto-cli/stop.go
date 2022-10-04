package main

type StopCommand struct {
	Args struct {
		Node Nodename
	} `positional-args:"yes" required:"yes"`
}

var stopCommand = &StopCommand{}

func (c *StopCommand) Execute([]string) error {
	n, err := c.Args.Node.GetNode()
	if err != nil {
		return err
	}

	return n.StopTracking()
}

func init() {
	parser.AddCommand("stop", "stops tracking on a specified node", "Stops the tracking on a specified node", stopCommand)
}
