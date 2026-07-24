package gpsreg

import (
	"fmt"
	"os"
	"strings"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/as"
	"github.com/jclark/satpulse/gps/internal/casic"
	"github.com/jclark/satpulse/gps/internal/nmea"
	"github.com/jclark/satpulse/gps/internal/nov"
	"github.com/jclark/satpulse/gps/internal/quectel"
	"github.com/jclark/satpulse/gps/internal/rtcm"
	"github.com/jclark/satpulse/gps/internal/sdbp"
	"github.com/jclark/satpulse/gps/internal/septentrio"
	"github.com/jclark/satpulse/gps/internal/sino"
	"github.com/jclark/satpulse/gps/internal/spartn"
	"github.com/jclark/satpulse/gps/internal/ubx"
	"github.com/jclark/satpulse/gps/internal/unc"
)

type Vendor int

// Vendors start at 1: Vendor(0) is an invalid value meaning "no vendor
// specified". It exists only at parse boundaries (ParseVendor("") and
// an absent TOML vendor key return it) and never reaches the create
// functions as a vendor.
const (
	VendorOther Vendor = iota + 1
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
	TagUBX                = ubx.Tag
	TagNMEA               = nmea.Tag
	TagRTCM               = rtcm.Tag
	TagSPARTN             = spartn.Tag
	TagSBF                = septentrio.Tag
	TagSeptentrioReply    = septentrio.TagReply
	TagCASICBin           = casic.Tag
	TagAllystarBin        = as.Tag
	TagSDBP               = sdbp.Tag
	TagUnicoreBin         = unc.TagBinary
	TagUnicoreAscii       = unc.TagAscii
	TagNovAtelBin         = nov.TagBinary
	TagNovAtelAscii       = nov.TagAscii
	TagNovAtelAbbrevAscii = nov.TagAbbrevAscii
)

// NMEAPacketFormat is the NMEA packet format, re-exported for callers that
// need to create NMEA packets without depending on gps/internal/nmea directly.
var NMEAPacketFormat = nmea.PacketFormat

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

// allVendors lists every valid vendor value, one per vendorNames entry.
var allVendors = func() []Vendor {
	vs := make([]Vendor, len(vendorNames))
	for i := range vendorNames {
		vs[i] = Vendor(i + 1)
	}
	return vs
}()

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
	septentrio.PacketFormat,
	septentrio.ReplyPacketFormat,
}

// allVendorPacketFormats maps each vendor to the packet formats they are known to use.
// NMEA and RTCM are added to these automatically, so they are not included here.
var allVendorPacketFormatsMap = map[Vendor][]gpsprot.PacketFormat{
	// no entry needed for VendorOther, since it is treated like vendors we do not currently support
	VendorAllystar:   {as.PacketFormat},
	VendorBynav:      {nov.BinPacketFormat, nov.AsciiPacketFormat, nov.AbbrevAsciiPacketFormat},
	VendorNovAtel:    {nov.BinPacketFormat, nov.AsciiPacketFormat, nov.AbbrevAsciiPacketFormat},
	VendorSeptentrio: {septentrio.PacketFormat, septentrio.ReplyPacketFormat},
	VendorSinoGNSS:   {nov.BinPacketFormat, nov.AsciiPacketFormat, nov.AbbrevAsciiPacketFormat},
	VendorTechtotop:  {sdbp.PacketFormat},
	VendorUblox:      {ubx.PacketFormat},
	VendorUnicore:    {unc.BinPacketFormat, unc.AsciiPacketFormat, nov.BinPacketFormat, nov.AsciiPacketFormat, nov.AbbrevAsciiPacketFormat},
	VendorZhongke:    {casic.PacketFormat},
}

// CreatePacketFormats returns the packet formats to scan for. NMEA and
// RTCM are always included; the vendor-specific formats are a walk of
// allVendorPacketFormats (which stays authoritative for scan order),
// keeping each format used by at least one of the given vendors per
// allVendorPacketFormatsMap. An empty list defaults to every vendor,
// so CreatePacketFormats(nil) reproduces the whole flat list in order.
// Membership is compared by Tag(), not interface equality: some
// PacketFormat implementations hold func fields, and comparing those
// panics.
func CreatePacketFormats(vendors []Vendor) []gpsprot.PacketFormat {
	if len(vendors) == 0 {
		vendors = allVendors
	}
	tags := make(map[gpsprot.Tag]struct{})
	for _, v := range vendors {
		for _, f := range allVendorPacketFormatsMap[v] {
			tags[f.Tag()] = struct{}{}
		}
	}
	formats := []gpsprot.PacketFormat{nmea.PacketFormat, rtcm.PacketFormat} // NMEA and RTCM are common to all vendors
	for _, f := range allVendorPacketFormats {
		if _, ok := tags[f.Tag()]; ok {
			formats = append(formats, f)
		}
	}
	return formats
}

// CreateCorrectionFormats returns the packet formats carried by a GNSS
// correction stream from a network source (Ntrip caster or TCP), as opposed
// to CreatePacketFormats, which autodetects the output of a connected
// receiver. SPARTN is included here, but is intentionally absent from the
// receiver autodetect set because its preamble is the common ASCII byte 's'.
func CreateCorrectionFormats() []gpsprot.PacketFormat {
	return []gpsprot.PacketFormat{rtcm.PacketFormat, spartn.PacketFormat}
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
	i := int(v) - 1
	if i < 0 || i >= len(vendorNames) {
		return fmt.Sprintf("Vendor(%d)", v)
	}
	return vendorNames[i]
}

// ParseVendor parses a vendor string and returns the corresponding Vendor.
// An empty string returns the zero Vendor, meaning no vendor specified.
// It returns an error if the string is not a recognized vendor name.
func ParseVendor(vendor string) (Vendor, error) {
	if vendor == "" {
		return 0, nil
	}
	if v, ok := vendorMap[strings.ToLower(vendor)]; ok {
		return v, nil
	}
	return 0, fmt.Errorf("unknown vendor: %q", vendor)
}

const envVendorsVar = "SATPULSE_VENDORS"

// EnvVendors parses the SATPULSE_VENDORS declaration of the vendors
// whose receivers may be attached to this machine. Unset or empty
// yields nil. "all" yields every valid vendor and must not be combined
// with other names. Otherwise the value is a comma-separated list of
// vendor names (aliases accepted, whitespace around names ignored); an
// empty element or unrecognized name is an error, and duplicates are
// dropped with order preserved. A non-nil error is fatal at startup.
func EnvVendors() ([]Vendor, error) {
	s := strings.TrimSpace(os.Getenv(envVendorsVar))
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	var vendors []Vendor
	seen := make(map[Vendor]struct{})
	for _, name := range parts {
		name = strings.TrimSpace(name)
		if strings.EqualFold(name, "all") {
			if len(parts) != 1 {
				return nil, fmt.Errorf("%s: \"all\" cannot be combined with other vendor names", envVendorsVar)
			}
			return append([]Vendor(nil), allVendors...), nil
		}
		if name == "" {
			return nil, fmt.Errorf("%s: empty vendor name", envVendorsVar)
		}
		v, err := ParseVendor(name)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", envVendorsVar, err)
		}
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			vendors = append(vendors, v)
		}
	}
	return vendors, nil
}

// UnmarshalText implements encoding.TextUnmarshaler for Vendor.
func (v *Vendor) UnmarshalText(data []byte) error {
	var err error
	*v, err = ParseVendor(string(data))
	return err
}

// CreatePacketProcessors creates packet processors for all registered protocols.
// A shared NavEpochManager coordinates epoch handling across protocols.
// The processor map is always complete; vendor-specific tuning
// (SetVendor: NMEA SV numbering, nov dialect) is applied only when
// exactly one vendor is given, so a singleton declaration acts like an
// explicit vendor everywhere.
func CreatePacketProcessors(vendors []Vendor) map[gpsprot.Tag]gpsprot.PacketProcessor {
	mgr := gpsprot.NewNavEpochManager()
	nmeaPP := nmea.NewPacketProcessor(mgr)
	nmeaPP.AddExtHandler(quectel.NewHandler())
	procs := map[gpsprot.Tag]gpsprot.PacketProcessor{
		ubx.Tag:            ubx.NewPacketProcessor(mgr),
		casic.Tag:          casic.NewPacketProcessor(mgr),
		as.Tag:             as.NewPacketProcessor(mgr),
		sdbp.Tag:           sdbp.NewPacketProcessor(mgr),
		nmea.Tag:           nmeaPP,
		rtcm.Tag:           rtcm.NewPacketProcessor(),
		unc.TagBinary:      unc.NewBinPacketProcessor(mgr),
		unc.TagAscii:       unc.NewAsciiPacketProcessor(mgr),
		nov.TagBinary:      nov.NewBinPacketProcessor(mgr),
		nov.TagAscii:       nov.NewAsciiPacketProcessor(mgr),
		nov.TagAbbrevAscii: nov.NewAbbrevAsciiPacketProcessor(),
		septentrio.Tag:     septentrio.NewPacketProcessor(mgr),
	}
	if len(vendors) == 1 {
		SetVendor(procs, vendors[0])
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

// CreateConfigProtocol returns the config protocol for vendor, or nil
// if it has none. This is the one place a new config protocol is wired
// in: each config branch adds one case.
func CreateConfigProtocol(vendor Vendor) gpsprot.ConfigProtocol {
	switch vendor {
	case VendorUblox:
		return ubx.NewConfigProtocol()
	case VendorUnicore:
		return unc.NewConfigProtocol()
	case VendorZhongke:
		return casic.NewConfigProtocol()
	default:
		return nil
	}
}

// defaultConfigProtocolVendors is the probe set used when no vendor is
// asserted: the non-experimental config protocols. A config protocol
// whose vendor is not listed here is experimental - present in the
// build but not probed by default. Graduation is adding the vendor.
var defaultConfigProtocolVendors = []Vendor{VendorUblox, VendorUnicore}

// CreateConfigProtocols returns the config protocols to probe with, one
// per given vendor that has a config protocol; the list's order is
// preserved. An empty list defaults to defaultConfigProtocolVendors, so
// with nothing asserted the probe order stays ubx then unc. A vendor
// with no config protocol contributes nothing, so an explicitly
// specified one yields an empty result: listen-only detection.
func CreateConfigProtocols(vendors []Vendor) []gpsprot.ConfigProtocol {
	if len(vendors) == 0 {
		vendors = defaultConfigProtocolVendors
	}
	var protos []gpsprot.ConfigProtocol
	for _, v := range vendors {
		if p := CreateConfigProtocol(v); p != nil {
			protos = append(protos, p)
		}
	}
	return protos
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
	for _, f := range CreatePacketFormats(nil) {
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
