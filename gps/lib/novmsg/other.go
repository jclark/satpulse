package novmsg

// Message IDs for known but unimplemented NovAtel-compatible binary messages.
const (
	BdsEphemerisID MsgID = 1696 // OEM7, Bynav, RTKLIB
	// BestGNSSPosID defined in nav.go (OEM7, Bynav)
	GalEphemerisID MsgID = 1122 // OEM6, Bynav, SinoGNSS, RTKLIB
	GloEphemerisID MsgID = 723  // OEM7, Bynav, SinoGNSS, RTKLIB
	GloRawEphemID  MsgID = 792  // OEM7, SinoGNSS
	GpsEphemID     MsgID = 7    // OEM7, Bynav
	HeadingID      MsgID = 971  // OEM6, Bynav, SinoGNSS
	Heading2ID     MsgID = 1335 // OEM7, Bynav, SinoGNSS
	MarkTimeID     MsgID = 231  // OEM6, Bynav, SinoGNSS
	// PsrDopID is defined in nav.go (has struct + registration)
	// PsrPosID is defined in nav.go (OEM7, SinoGNSS)
	// PsrVelID is defined in nav.go (OEM7, Bynav, SinoGNSS)
	QzssEphemerisID MsgID = 1336 // OEM7, Bynav
	RangeCmpID      MsgID = 140  // OEM7, Bynav, SinoGNSS, Unicore, RTKLIB
	RangeID         MsgID = 43   // OEM7, SinoGNSS, RTKLIB
	RawAlmID        MsgID = 74   // OEM7, SinoGNSS
	RawEphemID      MsgID = 41   // OEM7, SinoGNSS, RTKLIB
	RefStationID    MsgID = 175  // OEM7, SinoGNSS
	TrackStatID     MsgID = 83   // OEM7, Bynav
	VersionID       MsgID = 37   // OEM7, SinoGNSS
)

func init() {
	idNameMap[BdsEphemerisID] = "BDSEPHEMERIS"
	// BestGNSSPosID registered in nav.go init()
	idNameMap[GalEphemerisID] = "GALEPHEMERIS"
	idNameMap[GloEphemerisID] = "GLOEPHEMERIS"
	idNameMap[GloRawEphemID] = "GLORAWEPHEM"
	idNameMap[GpsEphemID] = "GPSEPHEM"
	idNameMap[HeadingID] = "HEADING"
	idNameMap[Heading2ID] = "HEADING2"
	idNameMap[MarkTimeID] = "MARKTIME"
	// PsrDopID registered in nav.go init()
	// PsrPosID registered in nav.go init()
	// PsrVelID registered in nav.go init()
	idNameMap[QzssEphemerisID] = "QZSSEPHEMERIS"
	idNameMap[RangeCmpID] = "RANGECMP"
	idNameMap[RangeID] = "RANGE"
	idNameMap[RawAlmID] = "RAWALM"
	idNameMap[RawEphemID] = "RAWEPHEM"
	idNameMap[RefStationID] = "REFSTATION"
	idNameMap[TrackStatID] = "TRACKSTAT"
	idNameMap[VersionID] = "VERSION"
}
