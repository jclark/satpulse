package gpsreg

import (
	"fmt"
	"strings"

	"github.com/jclark/satpulse/gps/internal/as"
	"github.com/jclark/satpulse/gps/internal/casic"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/nmea"
	"github.com/jclark/satpulse/gps/internal/nov"
	"github.com/jclark/satpulse/gps/internal/quectel"
	"github.com/jclark/satpulse/gps/internal/rtcm"
	"github.com/jclark/satpulse/gps/internal/sdbp"
	"github.com/jclark/satpulse/gps/internal/sino"
	"github.com/jclark/satpulse/gps/internal/ubx"
	"github.com/jclark/satpulse/gps/internal/unc"
)

// PacketFormats contains all known packet formats
var PacketFormats = []gpsprot.PacketFormat{
	ubx.PacketFormat,
	casic.PacketFormat,
	as.PacketFormat,
	sdbp.PacketFormat,
	nmea.PacketFormat,
	rtcm.PacketFormat,
	unc.BinPacketFormat,
	unc.AsciiPacketFormat,
	nov.BinPacketFormat,
	nov.AsciiPacketFormat,
}

type Vendor int

const (
	VendorUnknown Vendor = iota
	VendorAllystar
	VendorBynav
	VendorFuruno
	VendorMediaTek
	VendorNovAtel
	VendorQuectel
	VendorSeptentrio
	VendorSinoGNSS
	VendorSkyTraq
	VendorTaidou
	VendorTrimble
	VendorUblox
	VendorUnicore
	VendorZhongke
)

// Protocol tags for external use
const (
	TagUBX          = ubx.Tag
	TagNMEA         = nmea.Tag
	TagRTCM         = rtcm.Tag
	TagCASICBin     = casic.Tag
	TagAllystarBin  = as.Tag
	TagSDBP         = sdbp.Tag
	TagUnicoreBin   = unc.TagBinary
	TagUnicoreAscii = unc.TagAscii
	TagNovAtelBin   = nov.TagBinary
	TagNovAtelAscii = nov.TagAscii
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
	"Taidou",
	"Trimble",
	"u-blox",
	unc.Vendor,
	"Zhongke",
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

// CreatePacketProcessors creates packet processors for all registered protocols.
// A shared NavEpochManager coordinates epoch handling across protocols.
func CreatePacketProcessors(vendor Vendor) map[gpsprot.Tag]gpsprot.PacketProcessor {
	mgr := gpsprot.NewNavEpochManager()
	nmeaPP := nmea.NewPacketProcessor(mgr)
	nmeaPP.AddExtHandler(quectel.NewHandler())
	procs := map[gpsprot.Tag]gpsprot.PacketProcessor{
		ubx.Tag:       ubx.NewPacketProcessor(mgr),
		casic.Tag:     casic.NewPacketProcessor(mgr),
		as.Tag:        as.NewPacketProcessor(mgr),
		sdbp.Tag:      sdbp.NewPacketProcessor(mgr),
		nmea.Tag:      nmeaPP,
		rtcm.Tag:      rtcm.NewPacketProcessor(),
		unc.TagBinary: unc.NewBinPacketProcessor(mgr),
		unc.TagAscii:  unc.NewAsciiPacketProcessor(mgr),
		nov.TagBinary: nov.NewBinPacketProcessor(mgr),
		nov.TagAscii:  nov.NewAsciiPacketProcessor(mgr),
	}
	if vendor != VendorUnknown {
		SetVendor(procs, vendor)
	}
	return procs
}

type novVariantSetter interface {
	SetVariant(nov.Variant)
}

// SetVendor configures vendor-specific behavior on packet processors.
// Called by CreatePacketProcessors when vendor is known at construction,
// or separately when the vendor is determined later (e.g. desktop GUI).
func SetVendor(procs map[gpsprot.Tag]gpsprot.PacketProcessor, vendor Vendor) {
	if nmeaPP, ok := procs[nmea.Tag].(*nmea.PacketProcessor); ok {
		if numbering := FindNMEASVNumbering(vendor); numbering != nil {
			nmeaPP.SetSVNumbering(numbering)
		}
	}
	v := novVariantFor(vendor)
	for _, pp := range procs {
		if vs, ok := pp.(novVariantSetter); ok {
			vs.SetVariant(v)
		}
	}
}

func novVariantFor(v Vendor) nov.Variant {
	switch v {
	case VendorSinoGNSS:
		return nov.VariantSinoGNSS
	case VendorUnicore:
		return nov.VariantUnicore
	case VendorBynav:
		return nov.VariantByNav
	default:
		return nov.VariantOEM7
	}
}

// CreateConfigProtocols creates all available configuration protocols
func CreateConfigProtocols() []gpsprot.ConfigProtocol {
	return []gpsprot.ConfigProtocol{
		ubx.NewConfigProtocol(),
		unc.NewConfigProtocol(),
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
	case "taidou":
		return VendorTaidou
	case "trimble":
		return VendorTrimble
	case "u-blox", "ublox":
		return VendorUblox
	case "unicore":
		return VendorUnicore
	case "zhongke":
		return VendorZhongke
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
