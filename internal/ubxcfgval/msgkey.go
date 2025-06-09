package ubxcfgval

type KeyM uint16

type Port uint8

const (
	I2C Port = iota
	UART1
	UART2
	USB
	SPI
)

func (k KeyM) KeyU(port Port) KeyU {
	return KeyU(uint32(k) + uint32(port) + 0x20910000)
}

const KNmeaIdDtm KeyM = 0x0A6
const KNmeaIdGbs KeyM = 0x0DD
const KNmeaIdGga KeyM = 0x0BA
const KNmeaIdGll KeyM = 0x0C9
const KNmeaIdGns KeyM = 0x0B5
const KNmeaIdGrs KeyM = 0x0CE
const KNmeaIdGsa KeyM = 0x0BF
const KNmeaIdGst KeyM = 0x0D3
const KNmeaIdGsv KeyM = 0x0C4
const KNmeaIdRlm KeyM = 0x400
const KNmeaIdRmc KeyM = 0x0AB
const KNmeaIdVlw KeyM = 0x0E7
const KNmeaIdVtg KeyM = 0x0B0
const KNmeaIdZda KeyM = 0x0D8
const KNmeaNav2IdGga KeyM = 0x661
const KNmeaNav2IdGll KeyM = 0x670
const KNmeaNav2IdGns KeyM = 0x65C
const KNmeaNav2IdGsa KeyM = 0x666
const KNmeaNav2IdRmc KeyM = 0x652
const KNmeaNav2IdVtg KeyM = 0x657
const KNmeaNav2IdZda KeyM = 0x67F
const KPubxIdPolyp KeyM = 0x0EC
const KPubxIdPolys KeyM = 0x0F1
const KPubxIdPolyt KeyM = 0x0F6
const KRtcm3xType1005 KeyM = 0x2BD
const KRtcm3xType1074 KeyM = 0x35E
const KRtcm3xType1077 KeyM = 0x2CC
const KRtcm3xType1084 KeyM = 0x363
const KRtcm3xType1087 KeyM = 0x2D1
const KRtcm3xType1094 KeyM = 0x368
const KRtcm3xType1097 KeyM = 0x318
const KRtcm3xType1124 KeyM = 0x36D
const KRtcm3xType1127 KeyM = 0x2D6
const KRtcm3xType1230 KeyM = 0x303
const KRtcm3xType40720 KeyM = 0x2FE
const KRtcm3xType40721 KeyM = 0x381
const KUbxLogInfo KeyM = 0x259
const KUbxMonComms KeyM = 0x34F
const KUbxMonHw KeyM = 0x1B4
const KUbxMonHw2 KeyM = 0x1B9
const KUbxMonHw3 KeyM = 0x354
const KUbxMonIo KeyM = 0x1A5
const KUbxMonMsgpp KeyM = 0x196
const KUbxMonRf KeyM = 0x359
const KUbxMonRxbuf KeyM = 0x1A0
const KUbxMonRxr KeyM = 0x187
const KUbxMonSpan KeyM = 0x38B
const KUbxMonSys KeyM = 0x69D
const KUbxMonTxbuf KeyM = 0x19B
const KUbxNav2Clock KeyM = 0x430
const KUbxNav2Cov KeyM = 0x435
const KUbxNav2Dop KeyM = 0x465
const KUbxNav2Eoe KeyM = 0x565
const KUbxNav2Odo KeyM = 0x475
const KUbxNav2Posecef KeyM = 0x480
const KUbxNav2Posllh KeyM = 0x485
const KUbxNav2Pvt KeyM = 0x490
const KUbxNav2Sat KeyM = 0x495
const KUbxNav2Sbas KeyM = 0x500
const KUbxNav2Sig KeyM = 0x505
const KUbxNav2Status KeyM = 0x515
const KUbxNav2Svin KeyM = 0x520
const KUbxNav2Timebds KeyM = 0x525
const KUbxNav2Timegal KeyM = 0x530
const KUbxNav2Timeglo KeyM = 0x535
const KUbxNav2Timegps KeyM = 0x540
const KUbxNav2Timels KeyM = 0x545
const KUbxNav2Timenavic KeyM = 0x6A7
const KUbxNav2Timeutc KeyM = 0x550
const KUbxNav2Velecef KeyM = 0x555
const KUbxNav2Velned KeyM = 0x560
const KUbxNavClock KeyM = 0x065
const KUbxNavCov KeyM = 0x083
const KUbxNavDop KeyM = 0x038
const KUbxNavEoe KeyM = 0x15F
const KUbxNavGeofence KeyM = 0x0A1
const KUbxNavNmi KeyM = 0x590
const KUbxNavOdo KeyM = 0x07E
const KUbxNavOrb KeyM = 0x010
const KUbxNavPosecef KeyM = 0x024
const KUbxNavPosllh KeyM = 0x029
const KUbxNavPvt KeyM = 0x006
const KUbxNavSat KeyM = 0x015
const KUbxNavSbas KeyM = 0x06A
const KUbxNavSig KeyM = 0x345
const KUbxNavStatus KeyM = 0x01A
const KUbxNavSvin KeyM = 0x088
const KUbxNavTimebds KeyM = 0x051
const KUbxNavTimegal KeyM = 0x056
const KUbxNavTimeglo KeyM = 0x04C
const KUbxNavTimegps KeyM = 0x047
const KUbxNavTimels KeyM = 0x060
const KUbxNavTimenavic KeyM = 0x6A2
const KUbxNavTimeutc KeyM = 0x05B
const KUbxNavVelecef KeyM = 0x03D
const KUbxNavVelned KeyM = 0x042
const KUbxRxmCor KeyM = 0x6B6
const KUbxRxmMeasx KeyM = 0x204
const KUbxRxmRawx KeyM = 0x2A4
const KUbxRxmRlm KeyM = 0x25E
const KUbxRxmRtcm KeyM = 0x268
const KUbxRxmSfrbx KeyM = 0x231
const KUbxRxmTm KeyM = 0x610
const KUbxSecSig KeyM = 0x634
const KUbxSecSiglog KeyM = 0x689
const KUbxTimSvin KeyM = 0x097
const KUbxTimTm2 KeyM = 0x178
const KUbxTimTp KeyM = 0x17D
const KUbxTimVrfy KeyM = 0x092

var msgKeyNames = map[KeyM]string{
	KNmeaIdDtm:           "NMEA_ID_DTM",
	KNmeaIdGbs:           "NMEA_ID_GBS",
	KNmeaIdGga:           "NMEA_ID_GGA",
	KNmeaIdGll:           "NMEA_ID_GLL",
	KNmeaIdGns:           "NMEA_ID_GNS",
	KNmeaIdGrs:           "NMEA_ID_GRS",
	KNmeaIdGsa:           "NMEA_ID_GSA",
	KNmeaIdGst:           "NMEA_ID_GST",
	KNmeaIdGsv:           "NMEA_ID_GSV",
	KNmeaIdRlm:           "NMEA_ID_RLM",
	KNmeaIdRmc:           "NMEA_ID_RMC",
	KNmeaIdVlw:           "NMEA_ID_VLW",
	KNmeaIdVtg:           "NMEA_ID_VTG",
	KNmeaIdZda:           "NMEA_ID_ZDA",
	KNmeaNav2IdGga:       "NMEA_NAV2_ID_GGA",
	KNmeaNav2IdGll:       "NMEA_NAV2_ID_GLL",
	KNmeaNav2IdGns:       "NMEA_NAV2_ID_GNS",
	KNmeaNav2IdGsa:       "NMEA_NAV2_ID_GSA",
	KNmeaNav2IdRmc:       "NMEA_NAV2_ID_RMC",
	KNmeaNav2IdVtg:       "NMEA_NAV2_ID_VTG",
	KNmeaNav2IdZda:       "NMEA_NAV2_ID_ZDA",
	KPubxIdPolyp:         "PUBX_ID_POLYP",
	KPubxIdPolys:         "PUBX_ID_POLYS",
	KPubxIdPolyt:         "PUBX_ID_POLYT",
	KRtcm3xType1005:      "RTCM_3X_TYPE1005",
	KRtcm3xType1074:      "RTCM_3X_TYPE1074",
	KRtcm3xType1077:      "RTCM_3X_TYPE1077",
	KRtcm3xType1084:      "RTCM_3X_TYPE1084",
	KRtcm3xType1087:      "RTCM_3X_TYPE1087",
	KRtcm3xType1094:      "RTCM_3X_TYPE1094",
	KRtcm3xType1097:      "RTCM_3X_TYPE1097",
	KRtcm3xType1124:      "RTCM_3X_TYPE1124",
	KRtcm3xType1127:      "RTCM_3X_TYPE1127",
	KRtcm3xType1230:      "RTCM_3X_TYPE1230",
	KRtcm3xType40720:     "RTCM_3X_TYPE40720",
	KRtcm3xType40721:     "RTCM_3X_TYPE40721",
	KUbxLogInfo:          "UBX_LOG_INFO",
	KUbxMonComms:         "UBX_MON_COMMS",
	KUbxMonHw:            "UBX_MON_HW",
	KUbxMonHw2:           "UBX_MON_HW2",
	KUbxMonHw3:           "UBX_MON_HW3",
	KUbxMonIo:            "UBX_MON_IO",
	KUbxMonMsgpp:         "UBX_MON_MSGPP",
	KUbxMonRf:            "UBX_MON_RF",
	KUbxMonRxbuf:         "UBX_MON_RXBUF",
	KUbxMonRxr:           "UBX_MON_RXR",
	KUbxMonSpan:          "UBX_MON_SPAN",
	KUbxMonSys:           "UBX_MON_SYS",
	KUbxMonTxbuf:         "UBX_MON_TXBUF",
	KUbxNav2Clock:        "UBX_NAV2_CLOCK",
	KUbxNav2Cov:          "UBX_NAV2_COV",
	KUbxNav2Dop:          "UBX_NAV2_DOP",
	KUbxNav2Eoe:          "UBX_NAV2_EOE",
	KUbxNav2Odo:          "UBX_NAV2_ODO",
	KUbxNav2Posecef:      "UBX_NAV2_POSECEF",
	KUbxNav2Posllh:       "UBX_NAV2_POSLLH",
	KUbxNav2Pvt:          "UBX_NAV2_PVT",
	KUbxNav2Sat:          "UBX_NAV2_SAT",
	KUbxNav2Sbas:         "UBX_NAV2_SBAS",
	KUbxNav2Sig:          "UBX_NAV2_SIG",
	KUbxNav2Status:       "UBX_NAV2_STATUS",
	KUbxNav2Svin:         "UBX_NAV2_SVIN",
	KUbxNav2Timebds:      "UBX_NAV2_TIMEBDS",
	KUbxNav2Timegal:      "UBX_NAV2_TIMEGAL",
	KUbxNav2Timeglo:      "UBX_NAV2_TIMEGLO",
	KUbxNav2Timegps:      "UBX_NAV2_TIMEGPS",
	KUbxNav2Timels:       "UBX_NAV2_TIMELS",
	KUbxNav2Timenavic:    "UBX_NAV2_TIMENAVIC",
	KUbxNav2Timeutc:      "UBX_NAV2_TIMEUTC",
	KUbxNav2Velecef:      "UBX_NAV2_VELECEF",
	KUbxNav2Velned:       "UBX_NAV2_VELNED",
	KUbxNavClock:         "UBX_NAV_CLOCK",
	KUbxNavCov:           "UBX_NAV_COV",
	KUbxNavDop:           "UBX_NAV_DOP",
	KUbxNavEoe:           "UBX_NAV_EOE",
	KUbxNavGeofence:      "UBX_NAV_GEOFENCE",
	KUbxNavNmi:           "UBX_NAV_NMI",
	KUbxNavOdo:           "UBX_NAV_ODO",
	KUbxNavOrb:           "UBX_NAV_ORB",
	KUbxNavPosecef:       "UBX_NAV_POSECEF",
	KUbxNavPosllh:        "UBX_NAV_POSLLH",
	KUbxNavPvt:           "UBX_NAV_PVT",
	KUbxNavSat:           "UBX_NAV_SAT",
	KUbxNavSbas:          "UBX_NAV_SBAS",
	KUbxNavSig:           "UBX_NAV_SIG",
	KUbxNavStatus:        "UBX_NAV_STATUS",
	KUbxNavSvin:          "UBX_NAV_SVIN",
	KUbxNavTimebds:       "UBX_NAV_TIMEBDS",
	KUbxNavTimegal:       "UBX_NAV_TIMEGAL",
	KUbxNavTimeglo:       "UBX_NAV_TIMEGLO",
	KUbxNavTimegps:       "UBX_NAV_TIMEGPS",
	KUbxNavTimels:        "UBX_NAV_TIMELS",
	KUbxNavTimenavic:     "UBX_NAV_TIMENAVIC",
	KUbxNavTimeutc:       "UBX_NAV_TIMEUTC",
	KUbxNavVelecef:       "UBX_NAV_VELECEF",
	KUbxNavVelned:        "UBX_NAV_VELNED",
	KUbxRxmCor:           "UBX_RXM_COR",
	KUbxRxmMeasx:         "UBX_RXM_MEASX",
	KUbxRxmRawx:          "UBX_RXM_RAWX",
	KUbxRxmRlm:           "UBX_RXM_RLM",
	KUbxRxmRtcm:          "UBX_RXM_RTCM",
	KUbxRxmSfrbx:         "UBX_RXM_SFRBX",
	KUbxRxmTm:            "UBX_RXM_TM",
	KUbxSecSig:           "UBX_SEC_SIG",
	KUbxSecSiglog:        "UBX_SEC_SIGLOG",
	KUbxTimSvin:          "UBX_TIM_SVIN",
	KUbxTimTm2:           "UBX_TIM_TM2",
	KUbxTimTp:            "UBX_TIM_TP",
	KUbxTimVrfy:          "UBX_TIM_VRFY",
}

var portNames = []string{"I2C", "UART1", "UART2", "USB", "SPI"}

func MakeMsgoutGroup() map[string]Desc {
	msgout := make(map[string]Desc)
	for keyM, name := range msgKeyNames {
		for port := I2C; port <= SPI; port++ {
			itemName := name + "_" + portNames[port]
			msgout[itemName] = U(keyM.KeyU(port))
		}
	}
	return msgout
}

func NewSchemaWithMsgout(schema *Schema) *Schema {
	return schema.AddGroup("MSGOUT", MakeMsgoutGroup())
}
