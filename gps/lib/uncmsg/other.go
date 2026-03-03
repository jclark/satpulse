package uncmsg

// Message IDs for all other Unicore binary messages
const (
	// Navigation and position messages
	ADRNavID     MsgID = 142
	ADRNavHID    MsgID = 2117
	ADRDOPID     MsgID = 953
	ADRDOPHID    MsgID = 2121
	// BestNavID    MsgID = 2118 // implemented in nav.go
	BestNavHID   MsgID = 2119
	// BestNavXYZID MsgID = 240 // implemented in nav.go
	BestNavXYZHID MsgID = 242
	// BestSatID    MsgID = 1041 // implemented in sats.go
	BSLNEnuHD2ID MsgID = 1316
	BSLNXyzHD2ID MsgID = 1317
	DOPHD2ID     MsgID = 1333
	MSPosID      MsgID = 520
	PPPDOPID     MsgID = 1025
	// PPPNavID MsgID = 1026 // implemented in nav.go
	PVTSlnID     MsgID = 1021
	SPPDOPID     MsgID = 173
	SPPDOPHID    MsgID = 2120
	SPPNavID     MsgID = 46
	SPPNavHID    MsgID = 2116
	// StaDOPID   MsgID = 954 // implemented in dop.go
	StaDOPHID    MsgID = 2122
	UniHeadingID MsgID = 972
	UniHeading2ID MsgID = 1331

	// Ephemeris messages
	BD3EphID   MsgID = 2999
	BDSEphID   MsgID = 108
	GALEphID   MsgID = 109
	GLOEphID   MsgID = 107
	GPSEphID   MsgID = 106
	IRNSSEphID MsgID = 112
	QZSSEphID  MsgID = 110

	// Ionosphere and UTC messages
	BD3IonID MsgID = 21
	// BD3UTCID MsgID = 22 // implemented in time.go
	BDSIonID MsgID = 4
	// BDSUTCID MsgID = 2012 // implemented in time.go
	GALIonID MsgID = 9
	// GALUTCID MsgID = 20 // implemented in time.go
	GPSIonID MsgID = 8
	// GPSUTCID MsgID = 19 // implemented in time.go

	// Observation messages
	ObsvBaseID  MsgID = 284
	ObsvHID     MsgID = 13
	ObsvHCmpID  MsgID = 139
	ObsvMID     MsgID = 12
	ObsvMCmpID  MsgID = 138

	// Status messages
	AGCID            MsgID = 220
	AGNSSStatusID    MsgID = 512
	BaseInfoID       MsgID = 176
	BasePosID        MsgID = 49
	EnvInfoID        MsgID = 11779
	EventFlagID      MsgID = 312
	EventSlnID       MsgID = 311
	FreqJamStatusID  MsgID = 519
	HeadingStatusID  MsgID = 521
	HWStatusID       MsgID = 218
	JamStatusID      MsgID = 511
	// PPSStatusID MsgID = 9000 // implemented in time.go
	RTCMStatusID     MsgID = 2125
	RTKStatusID      MsgID = 509
	RTKStatus2ID     MsgID = 691
	RTCStatusID      MsgID = 510

	// PPP B2b messages
	PPPB2bInfo1ID MsgID = 2302
	PPPB2bInfo2ID MsgID = 2304
	PPPB2bInfo3ID MsgID = 2306
	PPPB2bInfo4ID MsgID = 2308
	PPPB2bInfo5ID MsgID = 2310
	PPPB2bInfo6ID MsgID = 2312
	PPPB2bInfo7ID MsgID = 2314

	// E6-HAS messages
	E6BiasBlockID      MsgID = 2323
	E6ClockFullBlockID MsgID = 2321
	E6ClockSubBlockID  MsgID = 2322
	E6MaskBlockID      MsgID = 2319
	E6OrbitBlockID     MsgID = 2320
	E6PBiasBlockID     MsgID = 2324

	// L6 MADOCA messages
	L6MDCType1ID MsgID = 2325
	L6MDCType2ID MsgID = 2326
	L6MDCType3ID MsgID = 2327
	L6MDCType4ID MsgID = 2328
	L6MDCType5ID MsgID = 2329
	L6MDCType7ID MsgID = 2330

	// Other messages
	AgricID      MsgID = 11276
	InfoPart1ID  MsgID = 1019
	InfoPart2ID  MsgID = 1020
	// RecTimeID MsgID = 102 // implemented in time.go
	SatECEFID    MsgID = 2115
	SatelliteID  MsgID = 1042
	// SatsInfoID MsgID = 2124 // implemented in sats.go
	TropInfoID   MsgID = 2318
	// VersionID MsgID = 37 // implemented in version.go
)

func init() {
	// Initialize idNameMap with all known message IDs
	// Navigation and position messages
	idNameMap[ADRNavID] = "ADRNAV"
	idNameMap[ADRNavHID] = "ADRNAVH"
	idNameMap[ADRDOPID] = "ADRDOP"
	idNameMap[ADRDOPHID] = "ADRDOPH"
	// BestNavID and BestNavXYZID registered via regMsg in nav.go
	idNameMap[BestNavHID] = "BESTNAVH"
	idNameMap[BestNavXYZHID] = "BESTNAVXYZH"
	idNameMap[BSLNEnuHD2ID] = "BSLNENUHD2"
	idNameMap[BSLNXyzHD2ID] = "BSLNXYZHD2"
	idNameMap[DOPHD2ID] = "DOPHD2"
	idNameMap[MSPosID] = "MSPOS"
	idNameMap[PPPDOPID] = "PPPDOP"
	// PPPNavID registered via regMsg in nav.go
	idNameMap[PVTSlnID] = "PVTSLN"
	idNameMap[SPPDOPID] = "SPPDOP"
	idNameMap[SPPDOPHID] = "SPPDOPH"
	idNameMap[SPPNavID] = "SPPNAV"
	idNameMap[SPPNavHID] = "SPPNAVH"
	// StaDOPID registered via regMsg in dop.go
	idNameMap[StaDOPHID] = "STADOPH"
	idNameMap[UniHeadingID] = "UNIHEADING"
	idNameMap[UniHeading2ID] = "UNIHEADING2"

	// Ephemeris messages
	idNameMap[BD3EphID] = "BD3EPH"
	idNameMap[BDSEphID] = "BDSEPH"
	idNameMap[GALEphID] = "GALEPH"
	idNameMap[GLOEphID] = "GLOEPH"
	idNameMap[GPSEphID] = "GPSEPH"
	idNameMap[IRNSSEphID] = "IRNSSEPH"
	idNameMap[QZSSEphID] = "QZSSEPH"

	// Ionosphere messages
	idNameMap[BD3IonID] = "BD3ION"
	idNameMap[BDSIonID] = "BDSION"
	idNameMap[GALIonID] = "GALION"
	idNameMap[GPSIonID] = "GPSION"

	// Observation messages
	idNameMap[ObsvBaseID] = "OBSVBASE"
	idNameMap[ObsvHID] = "OBSVH"
	idNameMap[ObsvHCmpID] = "OBSVHCMP"
	idNameMap[ObsvMID] = "OBSVM"
	idNameMap[ObsvMCmpID] = "OBSVMCMP"

	// Status messages
	idNameMap[AGCID] = "AGC"
	idNameMap[AGNSSStatusID] = "AGNSSSTATUS"
	idNameMap[BaseInfoID] = "BASEINFO"
	idNameMap[BasePosID] = "BASEPOS"
	idNameMap[EnvInfoID] = "ENVINFO"
	idNameMap[EventFlagID] = "EVENTFLAG"
	idNameMap[EventSlnID] = "EVENTSLN"
	idNameMap[FreqJamStatusID] = "FREQJAMSTATUS"
	idNameMap[HeadingStatusID] = "HEADINGSTATUS"
	idNameMap[HWStatusID] = "HWSTATUS"
	idNameMap[JamStatusID] = "JAMSTATUS"
	idNameMap[RTCMStatusID] = "RTCMSTATUS"
	idNameMap[RTKStatusID] = "RTKSTATUS"
	idNameMap[RTKStatus2ID] = "RTKSTATUS2"
	idNameMap[RTCStatusID] = "RTCSTATUS"

	// PPP B2b messages
	idNameMap[PPPB2bInfo1ID] = "PPPB2BINFO1"
	idNameMap[PPPB2bInfo2ID] = "PPPB2BINFO2"
	idNameMap[PPPB2bInfo3ID] = "PPPB2BINFO3"
	idNameMap[PPPB2bInfo4ID] = "PPPB2BINFO4"
	idNameMap[PPPB2bInfo5ID] = "PPPB2BINFO5"
	idNameMap[PPPB2bInfo6ID] = "PPPB2BINFO6"
	idNameMap[PPPB2bInfo7ID] = "PPPB2BINFO7"

	// E6-HAS messages
	idNameMap[E6BiasBlockID] = "E6CBIASBLOCK"
	idNameMap[E6ClockFullBlockID] = "E6CLOCKFULLBLOCK"
	idNameMap[E6ClockSubBlockID] = "E6CLOCKSUBBLOCK"
	idNameMap[E6MaskBlockID] = "E6MASKBLOCK"
	idNameMap[E6OrbitBlockID] = "E6ORBITBLOCK"
	idNameMap[E6PBiasBlockID] = "E6PBIASBLOCK"

	// L6 MADOCA messages
	idNameMap[L6MDCType1ID] = "L6MDCTYPE1"
	idNameMap[L6MDCType2ID] = "L6MDCTYPE2"
	idNameMap[L6MDCType3ID] = "L6MDCTYPE3"
	idNameMap[L6MDCType4ID] = "L6MDCTYPE4"
	idNameMap[L6MDCType5ID] = "L6MDCTYPE5"
	idNameMap[L6MDCType7ID] = "L6MDCTYPE7"

	// Other messages
	idNameMap[AgricID] = "AGRIC"
	idNameMap[InfoPart1ID] = "INFOPART1"
	idNameMap[InfoPart2ID] = "INFOPART2"
	idNameMap[SatECEFID] = "SATECEF"
	idNameMap[SatelliteID] = "SATELLITE"
	idNameMap[TropInfoID] = "TROPINFO"
}