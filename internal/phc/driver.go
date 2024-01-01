package phc

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

type DriverFlags uint32

const (
	DriverOneEdge DriverFlags = 1 << iota
	DriverBothEdges
	DriverKnown
	DriverPoll4Hz
	DriverCarrier             // PHC needs a carrier to work
	DriverEdges   DriverFlags = DriverOneEdge | DriverBothEdges
)

func (flags DriverFlags) Edges() int {
	return int(flags & DriverEdges)
}

func (flags DriverFlags) SetEdges(edges int) DriverFlags {
	if edges == 0 {
		return flags
	}
	if edges != 1 && edges != 2 {
		panic(fmt.Sprintf("invalid number of pulse edges %d", edges))
	}
	flags &^= DriverEdges
	return flags | DriverFlags(edges)
}

const cm4Flags DriverFlags = DriverKnown | DriverPoll4Hz | DriverCarrier
const intelFlags DriverFlags = DriverKnown | DriverBothEdges

func IfDriverFlags(ifname string) (DriverFlags, error) {
	name, err := IfDriverName(ifname)
	if err != nil {
		return 0, err
	}
	flags, lookAtPhy := IfDriverNameFlags(name)
	if flags != 0 || !lookAtPhy {
		return flags, nil
	}
	id, err := IfPhyID(ifname)
	if err != nil {
		return 0, err
	}
	return PhyIDDriverFlags(id), nil
}

// IfDriverNameFlags returns the driver flags for the given driver name.
// The second return value is true if we should look at the PHY ID to get the flags.
func IfDriverNameFlags(driverName string) (DriverFlags, bool) {
	switch driverName {
	case "igb", "igc":
		return intelFlags, false
	case "bcmgenet":
		return 0, true
	default:
		return 0, false
	}
}

func IfDriverName(ifname string) (name string, err error) {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, 0)
	if err != nil {
		return "", fmt.Errorf("could not create a socket: %w", err)
	}
	defer func() {
		err2 := unix.Close(fd)
		if err == nil {
			err = err2
		}
	}()
	drvInfo, err := unix.IoctlGetEthtoolDrvinfo(fd, ifname)
	if err != nil {
		err = fmt.Errorf("ETHTOOL_GDRVINFO %s: %w", ifname, err)
		return
	}
	s := drvInfo.Driver[:]
	i := bytes.IndexByte(s, 0)
	if i >= 0 {
		s = s[:i]
	}
	name = string(s)
	return
}

type PhyID uint32

func IfPhyID(ifname string) (PhyID, error) {
	path := fmt.Sprintf("/sys/class/net/%s/phydev/phy_id", ifname)
	data, err := os.ReadFile(path)
	if err != nil {
		// it's not really an error for the file not to exist
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	id, err := parsePhyID(string(data))
	if err != nil {
		return 0, fmt.Errorf("could not parse %s: %w", path, err)
	}
	return id, nil
}

const phyBCM54210E PhyID = 0x600d84a0
const bcmPhyIDMask PhyID = 0xFFFFFFF0

func PhyIDDriverFlags(id PhyID) DriverFlags {
	if id&bcmPhyIDMask == phyBCM54210E {
		return cm4Flags
	}
	return 0
}

func parsePhyID(s string) (PhyID, error) {
	var id uint32
	n, err := fmt.Sscanf(s, "0x%x\n", &id)
	if err != nil {
		return 0, err
	}
	if n != 1 {
		return 0, errors.New("expected hexadecimal number")
	}
	return PhyID(id), nil
}
