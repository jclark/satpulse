package asbin

// Message IDs for messages not implemented elsewhere
const (
	// NAV
	NavPvErrID   MsgID = clsNav | (0x26 << 8)
	NavSvStateID MsgID = clsNav | (0x32 << 8)
	NavNavPvtID  MsgID = clsNav | (0xC1 << 8)
	// CFG
	CfgDopID      MsgID = clsCfg | (0x0A << 8)
	CfgHeightID   MsgID = clsCfg | (0x0D << 8)
	CfgSbasID     MsgID = clsCfg | (0x0E << 8)
	CfgSpdHoldID  MsgID = clsCfg | (0x0F << 8)
	CfgEphSaveID  MsgID = clsCfg | (0x10 << 8)
	CfgNumSvID    MsgID = clsCfg | (0x11 << 8)
	CfgAntiJamID  MsgID = clsCfg | (0x15 << 8)
	CfgBdGeoID    MsgID = clsCfg | (0x16 << 8)
	CfgCarrSmthID MsgID = clsCfg | (0x17 << 8)
	CfgGeoFenceID MsgID = clsCfg | (0x18 << 8)
	CfgSleepID    MsgID = clsCfg | (0x41 << 8)
	CfgPwrCtlID   MsgID = clsCfg | (0x42 << 8)
	CfgPwr2CtlID  MsgID = clsCfg | (0x44 << 8)
	CfgFwUpID     MsgID = clsCfg | (0x50 << 8)
	CfgPvtLogID   MsgID = clsCfg | (0xB0 << 8) // removed in 2.3.2 spec
	// MON
	MonInfoID    MsgID = clsMon | (0x05 << 8)
	MonTrkChanID MsgID = clsMon | (0x08 << 8)
	MonRcvClkID  MsgID = clsMon | (0x09 << 8)
	MonCwiID     MsgID = clsMon | (0x0A << 8)
	// AID
	AidIniID      MsgID = clsAid | (0x01 << 8)
	AidPosID      MsgID = clsAid | (0x10 << 8)
	AidTimeID     MsgID = clsAid | (0x11 << 8)
	AidPalmGpsID  MsgID = clsAid | (0x22 << 8)
	AidPalmBdID   MsgID = clsAid | (0x23 << 8)
	AidPalmGlnID  MsgID = clsAid | (0x24 << 8)
	AidPalmGalID  MsgID = clsAid | (0x25 << 8)
	AidPalmQzssID MsgID = clsAid | (0x26 << 8)
	AidEphGpsID   MsgID = clsAid | (0x32 << 8)
	AidEphBdID    MsgID = clsAid | (0x33 << 8)
)

func init() {
	// NAV messages
	idNameMap[NavPvErrID] = "PVERR"
	idNameMap[NavSvStateID] = "SVSTATE"
	idNameMap[NavNavPvtID] = "NAVPVT"
	// CFG messages
	idNameMap[CfgDopID] = "DOP"
	idNameMap[CfgHeightID] = "HEIGHT"
	idNameMap[CfgSbasID] = "SBAS"
	idNameMap[CfgSpdHoldID] = "SPDHOLD"
	idNameMap[CfgEphSaveID] = "EPHSAVE"
	idNameMap[CfgNumSvID] = "NUMSV"
	idNameMap[CfgAntiJamID] = "ANTIJAM"
	idNameMap[CfgGeoFenceID] = "GEOFENCE"
	idNameMap[CfgCarrSmthID] = "CARRSMOOTH"
	idNameMap[CfgBdGeoID] = "BDGEO"
	idNameMap[CfgFwUpID] = "FWUP"
	idNameMap[CfgPvtLogID] = "PVTLOG"
	idNameMap[CfgPwrCtlID] = "PWRCTL"
	idNameMap[CfgSleepID] = "SLEEP"
	// MON messages
	idNameMap[MonInfoID] = "INFO"
	idNameMap[MonTrkChanID] = "TRKCHAN"
	idNameMap[MonRcvClkID] = "RCVCLK"
	idNameMap[MonCwiID] = "CWI"
	// AID messages
	idNameMap[AidIniID] = "INI"
	idNameMap[AidTimeID] = "TIME"
	idNameMap[AidPosID] = "POS"
	idNameMap[AidPalmGlnID] = "PALMGLN"
	idNameMap[AidPalmGpsID] = "PALMGPS"
	idNameMap[AidPalmBdID] = "PALMBD"
	idNameMap[AidPalmGalID] = "PALMGAL"
	idNameMap[AidPalmQzssID] = "PALMQZSS"
	idNameMap[AidEphGpsID] = "EPHGPS"
	idNameMap[AidEphBdID] = "EPHBD"
}
