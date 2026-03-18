package sdbpbin

// Message IDs for messages defined in the SDBP protocol spec but not
// implemented as parsed structs. These are registered in init() so that
// MsgID.String() returns human-readable names for packet logging.

const (
	// CFG messages not implemented
	CfgGNSS2ID    MsgID = clsCFG | (0x16 << 8)
	CfgBDSID      MsgID = clsCFG | (0x14 << 8)
	CfgBDSSVID    MsgID = clsCFG | (0x17 << 8)
	CfgSPIID      MsgID = clsCFG | (0x22 << 8)
	CfgIICID      MsgID = clsCFG | (0x23 << 8)
	CfgANTID      MsgID = clsCFG | (0x24 << 8)
	CfgDynamicID  MsgID = clsCFG | (0x31 << 8)
	CfgFixID      MsgID = clsCFG | (0x32 << 8)
	CfgSVID       MsgID = clsCFG | (0x33 << 8)
	CfgPVTID      MsgID = clsCFG | (0x34 << 8)
	CfgIntervalID MsgID = clsCFG | (0x35 << 8)
	CfgStaticID   MsgID = clsCFG | (0x37 << 8)
	CfgDTMID      MsgID = clsCFG | (0x38 << 8)
	CfgIonoID     MsgID = clsCFG | (0x39 << 8)
	CfgRTCM3ID    MsgID = clsCFG | (0x55 << 8)
	CfgGenID      MsgID = clsCFG | (0x60 << 8)
	CfgGen2ID     MsgID = clsCFG | (0x61 << 8)
	CfgUTCID      MsgID = clsCFG | (0x62 << 8)
	CfgFlashEphID MsgID = clsCFG | (0x64 << 8)

	// AST messages not implemented
	AstPosID   MsgID = clsAST | (0x01 << 8)
	AstTimeID  MsgID = clsAST | (0x02 << 8)
	AstTime2ID MsgID = clsAST | (0x03 << 8)
	AstBDSEID  MsgID = clsAST | (0x11 << 8)
	AstBDSAID  MsgID = clsAST | (0x12 << 8)
	AstGPSEID  MsgID = clsAST | (0x21 << 8)
	AstGPSAID  MsgID = clsAST | (0x22 << 8)
	AstGLOEID  MsgID = clsAST | (0x31 << 8)
	AstGLOAID  MsgID = clsAST | (0x32 << 8)
	AstGALEID  MsgID = clsAST | (0x41 << 8)
	AstGALAID  MsgID = clsAST | (0x42 << 8)
	AstEPOID   MsgID = clsAST | (0x50 << 8)

	// QUE messages not implemented
	QueVer2ID MsgID = clsQUE | (0x21 << 8)
	QueCIDID  MsgID = clsQUE | (0x03 << 8)

	// DAT messages not implemented
	DatLLAID   MsgID = clsDAT | (0x10 << 8)
	DatLLA2ID  MsgID = clsDAT | (0x1A << 8)
	DatECEFID  MsgID = clsDAT | (0x11 << 8)
	DatNEDID   MsgID = clsDAT | (0x12 << 8)
	DatNED2ID  MsgID = clsDAT | (0x1C << 8)
	DatCLKID   MsgID = clsDAT | (0x14 << 8)
	DatUTCTID  MsgID = clsDAT | (0x15 << 8)
	DatGLOTID  MsgID = clsDAT | (0x18 << 8)
	DatBDSEID  MsgID = clsDAT | (0x21 << 8)
	DatGPSEID  MsgID = clsDAT | (0x22 << 8)
	DatGLOEID  MsgID = clsDAT | (0x23 << 8)
	DatGALEID  MsgID = clsDAT | (0x26 << 8)
	DatBDSE2ID MsgID = clsDAT | (0x27 << 8)
	DatGPSE2ID MsgID = clsDAT | (0x28 << 8)
	DatBDSI2ID MsgID = clsDAT | (0x29 << 8)
	DatGPSIID  MsgID = clsDAT | (0x2A << 8)
	DatGALIID  MsgID = clsDAT | (0x2B << 8)
	DatSAT2ID  MsgID = clsDAT | (0x33 << 8)
	DatMEASXID MsgID = clsDAT | (0x31 << 8)
	DatSIG3ID  MsgID = clsDAT | (0x62 << 8)
)

func init() {
	// CFG
	idNameMap[CfgGNSS2ID] = "GNSS2"
	idNameMap[CfgBDSID] = "BDS"
	idNameMap[CfgBDSSVID] = "BDSSV"
	idNameMap[CfgSPIID] = "SPI"
	idNameMap[CfgIICID] = "IIC"
	idNameMap[CfgANTID] = "ANT"
	idNameMap[CfgDynamicID] = "DYNAMIC"
	idNameMap[CfgFixID] = "FIX"
	idNameMap[CfgSVID] = "SV"
	idNameMap[CfgPVTID] = "PVT"
	idNameMap[CfgIntervalID] = "INTERVAL"
	idNameMap[CfgStaticID] = "STATIC"
	idNameMap[CfgDTMID] = "DTM"
	idNameMap[CfgIonoID] = "IONO"
	idNameMap[CfgRTCM3ID] = "RTCM3"
	idNameMap[CfgGenID] = "GEN"
	idNameMap[CfgGen2ID] = "GEN2"
	idNameMap[CfgUTCID] = "UTC"
	idNameMap[CfgFlashEphID] = "FLASHEPH"
	// AST
	idNameMap[AstPosID] = "POS"
	idNameMap[AstTimeID] = "TIME"
	idNameMap[AstTime2ID] = "TIME2"
	idNameMap[AstBDSEID] = "BDSE"
	idNameMap[AstBDSAID] = "BDSA"
	idNameMap[AstGPSEID] = "GPSE"
	idNameMap[AstGPSAID] = "GPSA"
	idNameMap[AstGLOEID] = "GLOE"
	idNameMap[AstGLOAID] = "GLOA"
	idNameMap[AstGALEID] = "GALE"
	idNameMap[AstGALAID] = "GALA"
	idNameMap[AstEPOID] = "EPO"
	// QUE
	idNameMap[QueVer2ID] = "VER2"
	idNameMap[QueCIDID] = "CID"
	// DAT
	idNameMap[DatLLAID] = "LLA"
	idNameMap[DatLLA2ID] = "LLA2"
	idNameMap[DatECEFID] = "ECEF"
	idNameMap[DatNEDID] = "NED"
	idNameMap[DatNED2ID] = "NED2"
	idNameMap[DatCLKID] = "CLK"
	idNameMap[DatUTCTID] = "UTCT"
	idNameMap[DatGLOTID] = "GLOT"
	idNameMap[DatBDSEID] = "BDSE"
	idNameMap[DatGPSEID] = "GPSE"
	idNameMap[DatGLOEID] = "GLOE"
	idNameMap[DatGALEID] = "GALE"
	idNameMap[DatBDSE2ID] = "BDSE2"
	idNameMap[DatGPSE2ID] = "GPSE2"
	idNameMap[DatBDSI2ID] = "BDSI2"
	idNameMap[DatGPSIID] = "GPSI"
	idNameMap[DatGALIID] = "GALI"
	idNameMap[DatSAT2ID] = "SAT2"
	idNameMap[DatMEASXID] = "MEASX"
	idNameMap[DatSIG3ID] = "SIG3"
}
