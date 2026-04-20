package launchpad

import (
	"fmt"
	"strings"

	gomidi "gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
)

// OpenLaunchpad probes gomidi input ports for a Launchpad, finds the
// matching output, and returns a configured *Launchpad. Returns an error
// if no Launchpad is found. Callers should fall back to NewMock on error.
//
// Matches by lowercase substring "launchpad" in the port name; the output
// port name is derived by replacing "In" with "Out" in the input name.
func OpenLaunchpad() (*Launchpad, error) {
	inPorts := gomidi.GetInPorts()
	outPorts := gomidi.GetOutPorts()

	for _, inPort := range inPorts {
		inName := inPort.String()
		if !strings.Contains(strings.ToLower(inName), "launchpad") {
			continue
		}

		// Derive the output-port name from the input name: Novation drivers
		// expose paired ports that differ only by "In"/"Out". Fall back to
		// an exact-name match for drivers that use the same name on both
		// sides.
		outName := strings.Replace(inName, "In", "Out", 1)
		outPort := findOutPortByExactName(outPorts, outName)
		if outPort == nil {
			outPort = findOutPortByExactName(outPorts, inName)
		}

		return NewLaunchpad(inName, inPort, outPort)
	}

	return nil, fmt.Errorf("no launchpad found")
}

func findOutPortByExactName(outPorts gomidi.OutPorts, name string) drivers.Out {
	for _, p := range outPorts {
		if p.String() == name {
			return p
		}
	}
	return nil
}
