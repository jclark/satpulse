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
	return KeyU(uint32(k) + uint32(port) + 0X20910000)
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