package casbin

const (
	// CFG message IDs
	CfgPrtID     MsgID = clsCfg | (0x00 << 8)
	CfgMsgID     MsgID = clsCfg | (0x01 << 8)
	CfgRstID     MsgID = clsCfg | (0x02 << 8)
	CfgTPID      MsgID = clsCfg | (0x03 << 8)
	CfgRateID    MsgID = clsCfg | (0x04 << 8)
	CfgCfgID     MsgID = clsCfg | (0x05 << 8)
	CfgTModeID   MsgID = clsCfg | (0x06 << 8)
	CfgNavxID    MsgID = clsCfg | (0x07 << 8)
	CfgGroupID   MsgID = clsCfg | (0x08 << 8)
	CfgNavLimID  MsgID = clsCfg | (0x0A << 8)
	CfgNavModeID MsgID = clsCfg | (0x0B << 8)
	CfgNavFltID  MsgID = clsCfg | (0x0C << 8)
	CfgWnRefID   MsgID = clsCfg | (0x0D << 8)
	CfgIns2ID    MsgID = clsCfg | (0x0E << 8)
	CfgNavBandID MsgID = clsCfg | (0x0F << 8)
	CfgInsID     MsgID = clsCfg | (0x10 << 8)
	CfgCwiID     MsgID = clsCfg | (0x11 << 8)
	CfgNmeaID    MsgID = clsCfg | (0x12 << 8)
	CfgRtcmID    MsgID = clsCfg | (0x14 << 8)
	CfgTMode2ID  MsgID = clsCfg | (0x16 << 8)
	CfgSatMaskID MsgID = clsCfg | (0x21 << 8)
	CfgTgduID    MsgID = clsCfg | (0x22 << 8)
	CfgSbasID    MsgID = clsCfg | (0x23 << 8)
	// NAV message IDs (not implemented elsewhere)
	NavStatusID MsgID = clsNav | (0x00 << 8)
	NavDopID    MsgID = clsNav | (0x01 << 8)
	NavPvID     MsgID = clsNav | (0x03 << 8)
	NavImuAttID MsgID = clsNav | (0x06 << 8)
	// NAV2 message IDs (ZKW F8)
	Nav2StatusID  MsgID = clsNav2 | (0x00 << 8)
	Nav2DopID     MsgID = clsNav2 | (0x01 << 8)
	Nav2SolID     MsgID = clsNav2 | (0x02 << 8)
	Nav2PvhID     MsgID = clsNav2 | (0x03 << 8)
	Nav2SatID     MsgID = clsNav2 | (0x04 << 8)
	Nav2TimeUTCID MsgID = clsNav2 | (0x05 << 8)
	Nav2SigID     MsgID = clsNav2 | (0x06 << 8)
	Nav2ClkID     MsgID = clsNav2 | (0x07 << 8)
	Nav2RvtID     MsgID = clsNav2 | (0x08 << 8)
	Nav2RtcID     MsgID = clsNav2 | (0x09 << 8)
	// TIM2 message IDs (ZKW F8)
	Tim2TpxID     MsgID = clsTim2 | (0x00 << 8)
	Tim2TimeGPSID MsgID = clsTim2 | (0x01 << 8)
	Tim2TimeBDSID MsgID = clsTim2 | (0x02 << 8)
	Tim2TimeGLNID MsgID = clsTim2 | (0x03 << 8)
	Tim2TimeGALID MsgID = clsTim2 | (0x04 << 8)
	Tim2TimeIRNID MsgID = clsTim2 | (0x05 << 8)
	Tim2TimePosID MsgID = clsTim2 | (0x06 << 8)
	Tim2LsID      MsgID = clsTim2 | (0x07 << 8)
	Tim2LyID      MsgID = clsTim2 | (0x08 << 8)
	Tim2TcxoID    MsgID = clsTim2 | (0x09 << 8)
	// RXM message IDs
	RxmSensorID MsgID = clsRxm | (0x07 << 8)
	RxmMeasxID  MsgID = clsRxm | (0x10 << 8)
	RxmSvposID  MsgID = clsRxm | (0x11 << 8)
	// RXM2 message IDs (ZKW F8)
	Rxm2MeasxID MsgID = clsRxm2 | (0x00 << 8)
	Rxm2SvposID MsgID = clsRxm2 | (0x01 << 8)
	Rxm2SfrbxID MsgID = clsRxm2 | (0x06 << 8)
	Rxm2SvpID   MsgID = clsRxm2 | (0x0A << 8)
	// MON message IDs
	MonCwiID  MsgID = clsMon | (0x00 << 8)
	MonRfeID  MsgID = clsMon | (0x01 << 8)
	MonHistID MsgID = clsMon | (0x02 << 8)
	MonVerID  MsgID = clsMon | (0x04 << 8)
	MonCpuID  MsgID = clsMon | (0x05 << 8)
	MonIcvID  MsgID = clsMon | (0x06 << 8)
	MonModID  MsgID = clsMon | (0x07 << 8)
	MonHwID   MsgID = clsMon | (0x09 << 8)
	MonJsmID  MsgID = clsMon | (0x0A << 8)
	MonSecID  MsgID = clsMon | (0x0B << 8)
	// AID message IDs
	AidIniID MsgID = clsAid | (0x01 << 8)
	AidHuiID MsgID = clsAid | (0x03 << 8)
	// MSG message IDs (MsgBDSUTCID, MsgGPSUTCID implemented in msg.go)
	MsgBDSIonID MsgID = clsMsg | (0x01 << 8)
	MsgBDSEphID MsgID = clsMsg | (0x02 << 8)
	MsgGPSIonID MsgID = clsMsg | (0x06 << 8)
	MsgGPSEphID MsgID = clsMsg | (0x07 << 8)
	MsgGLNEphID MsgID = clsMsg | (0x08 << 8)
	// NMEA message IDs (for CFG-MSG rate configuration)
	NmeaGgaID MsgID = clsNmea | (0x00 << 8)
	NmeaGllID MsgID = clsNmea | (0x01 << 8)
	NmeaGsaID MsgID = clsNmea | (0x02 << 8)
	NmeaGsvID MsgID = clsNmea | (0x03 << 8)
	NmeaRmcID MsgID = clsNmea | (0x04 << 8)
	NmeaVtgID MsgID = clsNmea | (0x05 << 8)
	NmeaZdaID MsgID = clsNmea | (0x08 << 8)
)

func init() {
	// Register class names for new classes
	clsMap[clsNav2] = "NAV2"
	clsMap[clsTim2] = "TIM2"
	clsMap[clsRxm2] = "RXM2"
	clsMap[clsIns2] = "INS2"
	clsMap[clsRtcm2] = "RTCM"
	// CFG messages
	idNameMap[CfgPrtID] = "PRT"
	idNameMap[CfgMsgID] = "MSG"
	idNameMap[CfgRstID] = "RST"
	idNameMap[CfgTPID] = "TP"
	idNameMap[CfgRateID] = "RATE"
	idNameMap[CfgCfgID] = "CFG"
	idNameMap[CfgTModeID] = "TMODE"
	idNameMap[CfgNavxID] = "NAVX"
	idNameMap[CfgGroupID] = "GROUP"
	idNameMap[CfgNavLimID] = "NAVLIMIT"
	idNameMap[CfgNavModeID] = "NAVMODE"
	idNameMap[CfgNavFltID] = "NAVFLT"
	idNameMap[CfgWnRefID] = "WNREF"
	idNameMap[CfgIns2ID] = "INS2"
	idNameMap[CfgNavBandID] = "NAVBAND"
	idNameMap[CfgInsID] = "INS"
	idNameMap[CfgCwiID] = "CWI"
	idNameMap[CfgNmeaID] = "NMEA"
	idNameMap[CfgRtcmID] = "RTCM"
	idNameMap[CfgTMode2ID] = "TMODE2"
	idNameMap[CfgSatMaskID] = "SATMASK"
	idNameMap[CfgTgduID] = "TGDU"
	idNameMap[CfgSbasID] = "SBAS"
	// NAV messages
	idNameMap[NavStatusID] = "STATUS"
	idNameMap[NavDopID] = "DOP"
	idNameMap[NavPvID] = "PV"
	idNameMap[NavImuAttID] = "IMUATT"
	// NAV2 messages
	idNameMap[Nav2StatusID] = "STATUS"
	idNameMap[Nav2DopID] = "DOP"
	idNameMap[Nav2SolID] = "SOL"
	idNameMap[Nav2PvhID] = "PVH"
	idNameMap[Nav2SatID] = "SAT"
	idNameMap[Nav2TimeUTCID] = "TIMEUTC"
	idNameMap[Nav2SigID] = "SIG"
	idNameMap[Nav2ClkID] = "CLK"
	idNameMap[Nav2RvtID] = "RVT"
	idNameMap[Nav2RtcID] = "RTC"
	// TIM2 messages
	idNameMap[Tim2TpxID] = "TPX"
	idNameMap[Tim2TimeGPSID] = "TIMEGPS"
	idNameMap[Tim2TimeBDSID] = "TIMEBDS"
	idNameMap[Tim2TimeGLNID] = "TIMEGLN"
	idNameMap[Tim2TimeGALID] = "TIMEGAL"
	idNameMap[Tim2TimeIRNID] = "TIMEIRN"
	idNameMap[Tim2TimePosID] = "TIMEPOS"
	idNameMap[Tim2LsID] = "LS"
	idNameMap[Tim2LyID] = "LY"
	idNameMap[Tim2TcxoID] = "TCXO"
	// RXM messages
	idNameMap[RxmSensorID] = "SENSOR"
	idNameMap[RxmMeasxID] = "MEASX"
	idNameMap[RxmSvposID] = "SVPOS"
	// RXM2 messages
	idNameMap[Rxm2MeasxID] = "MEASX"
	idNameMap[Rxm2SvposID] = "SVPOS"
	idNameMap[Rxm2SfrbxID] = "SFRBX"
	idNameMap[Rxm2SvpID] = "SVP"
	// MON messages
	idNameMap[MonCwiID] = "CWI"
	idNameMap[MonRfeID] = "RFE"
	idNameMap[MonHistID] = "HIST"
	idNameMap[MonVerID] = "VER"
	idNameMap[MonCpuID] = "CPU"
	idNameMap[MonIcvID] = "ICV"
	idNameMap[MonModID] = "MOD"
	idNameMap[MonHwID] = "HW"
	idNameMap[MonJsmID] = "JSM"
	idNameMap[MonSecID] = "SEC"
	// AID messages
	idNameMap[AidIniID] = "INI"
	idNameMap[AidHuiID] = "HUI"
	// MSG messages (MsgBDSUTCID, MsgGPSUTCID registered in msg.go)
	idNameMap[MsgBDSIonID] = "BDSION"
	idNameMap[MsgBDSEphID] = "BDSEPH"
	idNameMap[MsgGPSIonID] = "GPSION"
	idNameMap[MsgGPSEphID] = "GPSEPH"
	idNameMap[MsgGLNEphID] = "GLNEPH"
	// NMEA messages
	idNameMap[NmeaGgaID] = "GGA"
	idNameMap[NmeaGllID] = "GLL"
	idNameMap[NmeaGsaID] = "GSA"
	idNameMap[NmeaGsvID] = "GSV"
	idNameMap[NmeaRmcID] = "RMC"
	idNameMap[NmeaVtgID] = "VTG"
	idNameMap[NmeaZdaID] = "ZDA"
}
