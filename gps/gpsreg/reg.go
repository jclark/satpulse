package gpsreg

import (
	"fmt"
	"strings"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/as"
	"github.com/jclark/satpulse/gps/internal/casic"
	"github.com/jclark/satpulse/gps/internal/nmea"
	"github.com/jclark/satpulse/gps/internal/nov"
	"github.com/jclark/satpulse/gps/internal/quectel"
	"github.com/jclark/satpulse/gps/internal/rtcm"
	"github.com/jclark/satpulse/gps/internal/sdbp"
	"github.com/jclark/satpulse/gps/internal/sino"
	"github.com/jclark/satpulse/gps/internal/ubx"
	"github.com/jclark/satpulse/gps/internal/unc"
)

type Vendor int

const (
	VendorUnknown Vendor = iota
	VendorOther
	VendorAllystar
	VendorBynav
	VendorFuruno
	VendorMediaTek
	VendorNovAtel
	VendorQuectel
	VendorSeptentrio
	VendorSinoGNSS
	VendorSkyTraq
	VendorTechtotop
	VendorTrimble
	VendorUblox
	VendorUnicore
	VendorZhongke
)

// Protocol tags for external use
const (
	TagUBX           = ubx.Tag
	TagNMEA          = nmea.Tag
	TagRTCM          = rtcm.Tag
	TagCASICBin      = casic.Tag
	TagAllystarBin   = as.Tag
	TagSDBP          = sdbp.Tag
	TagUnicoreBin    = unc.TagBinary
	TagUnicoreAscii  = unc.TagAscii
	TagNovAtelBin    = nov.TagBinary
	TagNovAtelAscii  = nov.TagAscii
	TagNovAtelAbbrevAscii = nov.TagAbbrevAscii
)

// RTCMPacketFormat is the RTCM packet format, re-exported for
// callers that need to scan RTCM without depending on
// gps/internal/rtcm directly.
var RTCMPacketFormat = rtcm.PacketFormat

var vendorNames = []string{
	"other",
	"Allystar",
	"Bynav",
	"Furuno",
	"MediaTek",
	"NovAtel",
	"Quectel",
	"Septentrio",
	"SinoGNSS",
	"SkyTraq",
	"Techtotop",
	"Trimble",
	"u-blox",
	unc.Vendor,
	"Zhongke",
}

// allVendorPacketFormats contains all vendor-specific (non-NMEA, non-RTCM) packet formats.
var allVendorPacketFormats = []gpsprot.PacketFormat{
	ubx.PacketFormat,
	casic.PacketFormat,
	as.PacketFormat,
	sdbp.PacketFormat,
	unc.BinPacketFormat,
	unc.AsciiPacketFormat,
	nov.BinPacketFormat,
	nov.AsciiPacketFormat,
	nov.AbbrevAsciiPacketFormat,
}

// allVendorPacketFormats maps each vendor to the packet formats they are known to use.
// NMEA and RTCM are added to these automatically, so they are not included here.
var allVendorPacketFormatsMap = map[Vendor][]gpsprot.PacketFormat{
	VendorUnknown: allVendorPacketFormats,
	// no entry needed for VendorOther, since it is treated like vendors we do not currently support
	VendorAllystar:  {as.PacketFormat},
	VendorBynav:     {nov.BinPacketFormat, nov.AsciiPacketFormat, nov.AbbrevAsciiPacketFormat},
	VendorNovAtel:   {nov.BinPacketFormat, nov.AsciiPacketFormat, nov.AbbrevAsciiPacketFormat},
	VendorSinoGNSS:  {nov.BinPacketFormat, nov.AsciiPacketFormat, nov.AbbrevAsciiPacketFormat},
	VendorTechtotop: {sdbp.PacketFormat},
	VendorUblox:     {ubx.PacketFormat},
	VendorUnicore:   {unc.BinPacketFormat, unc.AsciiPacketFormat, nov.BinPacketFormat, nov.AsciiPacketFormat, nov.AbbrevAsciiPacketFormat},
	VendorZhongke:   {casic.PacketFormat},
}

func CreatePacketFormats(vendor Vendor) []gpsprot.PacketFormat {
	formats := []gpsprot.PacketFormat{nmea.PacketFormat, rtcm.PacketFormat} // NMEA and RTCM are common to all vendors
	if vendorFormats, ok := allVendorPacketFormatsMap[vendor]; ok {
		formats = append(formats, vendorFormats...)
	}
	return formats
}

var vendorMap = func() map[string]Vendor {
	m := make(map[string]Vendor)
	for i, name := range vendorNames {
		m[strings.ToLower(name)] = Vendor(i + 1)
	}
	m["comnav"] = VendorSinoGNSS
	m["taidou"] = VendorTechtotop
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

// ParseVendor parses a vendor string and returns the corresponding Vendor.
// An empty string returns VendorUnknown.
// It returns an error if the string is not a recognized vendor name.
func ParseVendor(vendor string) (Vendor, error) {
	if vendor == "" {
		return VendorUnknown, nil
	}
	if v, ok := vendorMap[strings.ToLower(vendor)]; ok {
		return v, nil
	}
	return VendorUnknown, fmt.Errorf("unknown vendor: %q", vendor)
}

// UnmarshalText implements encoding.TextUnmarshaler for Vendor.
func (v *Vendor) UnmarshalText(data []byte) error {
	var err error
	*v, err = ParseVendor(string(data))
	return err
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
		nov.TagAbbrevAscii: nov.NewAbbrevAsciiPacketProcessor(),
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

// CreateConfigProtocols creates configuration protocols appropriate for the vendor.
// VendorUnknown returns all protocols. A specific vendor returns only matching ones.
func CreateConfigProtocols(vendor Vendor) []gpsprot.ConfigProtocol {
	switch vendor {
	case VendorUnknown:
		return []gpsprot.ConfigProtocol{
			ubx.NewConfigProtocol(),
			unc.NewConfigProtocol(),
		}
	case VendorUblox:
		return []gpsprot.ConfigProtocol{ubx.NewConfigProtocol()}
	case VendorUnicore:
		return []gpsprot.ConfigProtocol{unc.NewConfigProtocol()}
	default:
		return nil
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

var protocolMap = func() map[string]gpsprot.Tag {
	m := make(map[string]gpsprot.Tag)
	for _, f := range CreatePacketFormats(VendorUnknown) {
		tag := f.Tag()
		m[strings.ToUpper(string(tag))] = tag
	}
	return m
}()

// Tag returns the protocol tag.
func (prot Protocol) Tag() gpsprot.Tag {
	return gpsprot.Tag(prot)
}

// UnmarshalText implements encoding.TextUnmarshaler for Protocol.
func (prot *Protocol) UnmarshalText(data []byte) error {
	s := string(data)
	if s == "" {
		*prot = Protocol("")
		return nil
	}
	if tag, ok := protocolMap[strings.ToUpper(s)]; ok {
		*prot = Protocol(tag)
		return nil
	}
	return fmt.Errorf("unknown protocol: %s", s)
}
