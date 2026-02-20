package novmsg

// Message IDs for known but unimplemented NovAtel-compatible binary messages.
const (
	VersionID      MsgID = 37   // OEM7, SinoGNSS
	RawEphemID     MsgID = 41   // OEM7, SinoGNSS, RTKLIB
	RangeID        MsgID = 43   // OEM7, SinoGNSS, RTKLIB
	PsrPosID       MsgID = 47   // OEM7, SinoGNSS
	BestVelID      MsgID = 99   // OEM7, SinoGNSS
	PsrVelID       MsgID = 100  // OEM7, Bynav, SinoGNSS
	RangeCmpID     MsgID = 140  // OEM7, Bynav, SinoGNSS, Unicore, RTKLIB
	PsrDopID       MsgID = 174  // OEM7, SinoGNSS
	RefStationID   MsgID = 175  // OEM7, SinoGNSS
	MarkTimeID     MsgID = 231  // OEM6, Bynav, SinoGNSS
	GloEphemerisID MsgID = 723  // OEM7, Bynav, SinoGNSS, RTKLIB
	HeadingID      MsgID = 971  // OEM6, Bynav, SinoGNSS
	GalEphemerisID MsgID = 1122 // OEM6, Bynav, SinoGNSS, RTKLIB
	BdsEphemerisID MsgID = 1696 // OEM7, Bynav, RTKLIB
)

func init() {
	idNameMap[VersionID] = "VERSION"
	idNameMap[RawEphemID] = "RAWEPHEM"
	idNameMap[RangeID] = "RANGE"
	idNameMap[PsrPosID] = "PSRPOS"
	idNameMap[BestVelID] = "BESTVEL"
	idNameMap[PsrVelID] = "PSRVEL"
	idNameMap[RangeCmpID] = "RANGECMP"
	idNameMap[PsrDopID] = "PSRDOP"
	idNameMap[RefStationID] = "REFSTATION"
	idNameMap[MarkTimeID] = "MARKTIME"
	idNameMap[GloEphemerisID] = "GLOEPHEMERIS"
	idNameMap[HeadingID] = "HEADING"
	idNameMap[GalEphemerisID] = "GALEPHEMERIS"
	idNameMap[BdsEphemerisID] = "BDSEPHEMERIS"
}
