package novmsg

// Message IDs for known but unimplemented NovAtel-compatible binary messages.
const (
	BdsEphemerisID MsgID = 1696 // OEM7, Bynav, RTKLIB
	BestGNSSPosID  MsgID = 1429 // OEM7, Bynav
	BestGNSSVelID  MsgID = 1430 // OEM7, Bynav
	BestVelID      MsgID = 99   // OEM7, SinoGNSS
	GalEphemerisID MsgID = 1122 // OEM6, Bynav, SinoGNSS, RTKLIB
	GloEphemerisID MsgID = 723  // OEM7, Bynav, SinoGNSS, RTKLIB
	HeadingID      MsgID = 971  // OEM6, Bynav, SinoGNSS
	MarkTimeID     MsgID = 231  // OEM6, Bynav, SinoGNSS
	PsrDopID       MsgID = 174  // OEM7, SinoGNSS
	PsrPosID       MsgID = 47   // OEM7, SinoGNSS
	PsrVelID       MsgID = 100  // OEM7, Bynav, SinoGNSS
	RangeCmpID     MsgID = 140  // OEM7, Bynav, SinoGNSS, Unicore, RTKLIB
	RangeID        MsgID = 43   // OEM7, SinoGNSS, RTKLIB
	RawEphemID     MsgID = 41   // OEM7, SinoGNSS, RTKLIB
	RefStationID   MsgID = 175  // OEM7, SinoGNSS
	VersionID      MsgID = 37   // OEM7, SinoGNSS
)

func init() {
	idNameMap[BdsEphemerisID] = "BDSEPHEMERIS"
	idNameMap[BestGNSSPosID] = "BESTGNSSPOS"
	idNameMap[BestGNSSVelID] = "BESTGNSSVEL"
	idNameMap[BestVelID] = "BESTVEL"
	idNameMap[GalEphemerisID] = "GALEPHEMERIS"
	idNameMap[GloEphemerisID] = "GLOEPHEMERIS"
	idNameMap[HeadingID] = "HEADING"
	idNameMap[MarkTimeID] = "MARKTIME"
	idNameMap[PsrDopID] = "PSRDOP"
	idNameMap[PsrPosID] = "PSRPOS"
	idNameMap[PsrVelID] = "PSRVEL"
	idNameMap[RangeCmpID] = "RANGECMP"
	idNameMap[RangeID] = "RANGE"
	idNameMap[RawEphemID] = "RAWEPHEM"
	idNameMap[RefStationID] = "REFSTATION"
	idNameMap[VersionID] = "VERSION"
}
