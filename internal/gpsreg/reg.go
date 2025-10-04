package gpsreg

import (
	"fmt"
	"strings"

	"github.com/jclark/satpulse/internal/as"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/nmea"
	"github.com/jclark/satpulse/internal/rtcm"
	"github.com/jclark/satpulse/internal/sino"
	"github.com/jclark/satpulse/internal/ubx"
)

// PacketFormats contains all known packet formats
var PacketFormats = []gpsprot.PacketFormat{
	ubx.PacketFormat,
	nmea.PacketFormat,
	rtcm.PacketFormat,
}

type Vendor int

const (
	VendorUnknown     Vendor = iota
	VendorAllystar
	VendorBynav
	VendorFuruno
	VendorMediaTek
	VendorNovAtel
	VendorQuectel
	VendorSeptentrio
	VendorSinoGNSS
	VendorSkyTraq
	VendorTrimble
	VendorUblox
	VendorUnicore
)

var vendorNames = []string{
	"Allystar",
	"Bynav",
	"Furuno",
	"MediaTek",
	"NovAtel",
	"Quectel",
	"Septentrio",
	"SinoGNSS",
	"SkyTraq",
	"Trimble",
	"u-blox",
	"Unicore",
}

var vendorMap = func() map[string]Vendor {
	m := make(map[string]Vendor)
	for i, name := range vendorNames {
		m[strings.ToLower(name)] = Vendor(i + 1)
	}
	m["comnav"] = VendorSinoGNSS
	m["ublox"] = VendorUblox
	return m
}()

// String returns the string representation of the vendor
func (v Vendor) String() string {
	if v == VendorUnknown {
		return "Unknown"
	}
	i := int(v) - 1
	if i < 0 || i >= len(vendorNames) {
		return fmt.Sprintf("Vendor(%d)", v)
	}
	return vendorNames[i]
}

// ParseVendor parses a vendor string and returns the corresponding Vendor
func ParseVendor(vendor string) Vendor {
	if v, ok := vendorMap[strings.ToLower(vendor)]; ok {
		return v
	}
	return VendorUnknown
}

// CreatePacketProcessors creates packet processors for all registered protocols
func CreatePacketProcessors(nmeaNumbering []gpsprot.NMEASVNumberingRange) map[gpsprot.Tag]gpsprot.PacketProcessor {
	nmeaPP := nmea.NewPacketProcessor()
	if nmeaNumbering != nil {
		nmeaPP.SetSVNumbering(nmeaNumbering)
	}
	return map[gpsprot.Tag]gpsprot.PacketProcessor{
		ubx.Tag:  ubx.NewPacketProcessor(),
		nmea.Tag: nmeaPP,
		rtcm.Tag: rtcm.NewPacketProcessor(),
	}
}

// CreateConfigProtocols creates all available configuration protocols
func CreateConfigProtocols() []gpsprot.ConfigProtocol {
	return []gpsprot.ConfigProtocol{
		ubx.NewConfigProtocol(),
	}
}

func MakeVendor(vendor string) Vendor {
	switch strings.ToLower(vendor) {
	case "allystar":
		return VendorAllystar
	case "bynav":
		return VendorBynav
	case "furuno":
		return VendorFuruno
	case "mediatek":
		return VendorMediaTek
	case "novatel":
		return VendorNovAtel
	case "quectel":
		return VendorQuectel
	case "septentrio":
		return VendorSeptentrio
	case "sinognss":
		return VendorSinoGNSS
	case "skytraq":
		return VendorSkyTraq
	case "trimble":
		return VendorTrimble
	case "u-blox", "ublox":
		return VendorUblox
	case "unicore":
		return VendorUnicore
	default:
		return VendorUnknown
	}
}

func FindNMEASVNumbering(vendor Vendor) []gpsprot.NMEASVNumberingRange {
	switch vendor {
	case VendorAllystar:
		return as.NewNMEASVNumbering()
	case VendorSinoGNSS:
		return sino.NewNMEASVNumbering()
	case VendorUblox:
		return ubx.NewNMEASVNumbering()
	default:
		return nil
	}
}

// Protocol is like gpsprot.Tag but implements encoding.TextUnmarshaler
// and enforces that the tag is one of the known protocols.
type Protocol gpsprot.Tag

func (prot Protocol) Tag() gpsprot.Tag {
	return gpsprot.Tag(prot)
}

func (prot *Protocol) UnmarshalText(data []byte) error {
	s := string(data)
	switch strings.ToUpper(s) {
	case "UBX":
		*prot = Protocol(ubx.Tag)
	case "NMEA":
		*prot = Protocol(nmea.Tag)
	case "RTCM":
		*prot = Protocol(rtcm.Tag)
	case "":
		*prot = Protocol("")
	default:
		return fmt.Errorf("unknown protocol: %s", s)
	}
	return nil
}
