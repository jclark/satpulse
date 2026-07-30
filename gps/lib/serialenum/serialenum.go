// Package serialenum enumerates serial ports with human-readable display names.
package serialenum

import (
	"strconv"
	"strings"
)

// USBID identifies a USB product.
type USBID struct {
	VID uint16 `json:"vid"`
	PID uint16 `json:"pid"`
}

// Port describes a serial port for display in a dropdown or CLI listing.
type Port struct {
	Device  string `json:"device"`       // canonical device node to open
	Display string `json:"display"`      // human-readable label
	USB     USBID  `json:"usb,omitzero"` // USB vendor and product IDs
}

// enumeratorDisplay preserves the display formatting used with go.bug.st's
// platform-dependent Product field.
func enumeratorDisplay(name, product string, vid, pid uint16) string {
	tag := ubloxTag(vid, pid)
	if product != "" {
		if tag != "" {
			return product + " - " + tag
		}
		return product
	}
	if tag != "" {
		return name + " (" + tag + ")"
	}
	return name
}

// ubloxTag returns a u-blox generation tag from USB VID/PID, or empty string.
func ubloxTag(vid, pid uint16) string {
	if vid != 0x1546 {
		return ""
	}
	if pid >= 0x01A4 && pid <= 0x01AF {
		return "u-blox gen " + strconv.Itoa(int(pid-0x01A0))
	}
	return "u-blox"
}

func parseUSBID(s string) (uint16, error) {
	if s = strings.TrimSpace(s); s == "" {
		return 0, nil
	}
	n, err := strconv.ParseUint(s, 16, 16)
	if err != nil {
		return 0, err
	}
	return uint16(n), nil
}

// parseEnumeratorUSBID converts the optional ID strings supplied by
// go.bug.st. Incomplete or malformed metadata does not prevent enumeration.
func parseEnumeratorUSBID(vid, pid string) USBID {
	if strings.TrimSpace(vid) == "" || strings.TrimSpace(pid) == "" {
		return USBID{}
	}
	vidVal, err := parseUSBID(vid)
	if err != nil {
		return USBID{}
	}
	pidVal, err := parseUSBID(pid)
	if err != nil {
		return USBID{}
	}
	return USBID{VID: vidVal, PID: pidVal}
}
