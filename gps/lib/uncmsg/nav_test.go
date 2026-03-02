package uncmsg

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/jclark/satpulse/gps/lib/novmsg"
)

var bestNavTests = []dataTestCase{
	{
		name:      "BESTNAV with SINGLE fix",
		binPacket: mustHexDecode("aa44b5614608780000a06609984487058c4400000012160000000000100000002dde9be3b2762b4026ee7829432959400000f8a3039e2040d1a5f6c13d0000001503b63f2b5aa23fdac5114000000000000000000000a040341c1c00011211510000000008000000000000000000000086c53f34760b763f07a482eb9c676c409210d192ce1852bf2925963c1a3c5b3cab147aeb"),
		asciiPacket: "#BESTNAVA,97,GPS,FINE,2406,92751000,17548,0,18,22;SOL_COMPUTED,SINGLE,13.73183356550,100.64472424326,8.3086,-30.8310,WGS84,1.4220,1.2684,2.2777,\"\",0.000,5.000,52,28,28,0,1,12,11,51,SOL_COMPUTED,DOPPLER_VELOCITY,0.000,0.000,0.0054,227.237905,-0.0011,0.0183,0.0134*a10b7a38\r\n",
		msg: &Msg{
			Hdr: MsgHdr{
				CPUIdlePercent: 97,
				TimingHdr: TimingHdr{
					TimeRef:            TimeRefGPS,
					TimeStatus:         TimeStatusFine,
					Week:               2406,
					MillisecondsOfWeek: 92751000,
					Reserved:           17548,
					Version:            0,
					LeapSec:            18,
					DelayMs:            22,
				},
			},
			Body: &BestNav{
				Pos: novmsg.Pos[SolStatus, PosVelType]{
					PSolStatus:    SolComputed,
					PosType:       PosVelSingle,
					Lat:           13.731833565499153,
					Lon:           100.64472424325558,
					Hgt:           8.308621524833143,
					Undulation:    -30.830965042114258,
					DatumID:       DatumWGS84,
					LatSigma:      1.4219690561294556,
					LonSigma:      1.2683767080307007,
					HgtSigma:      2.277700901031494,
					StnID:         StationID{},
					DiffAge:       0.0,
					SolAge:        5.0,
					NumSVs:        52,
					NumSolnSVs:    28,
					NumSolnL1SVs:  28,
					NumSolnMulti:  0,
					Reserved:      1,
					ExtSolStat:    0x12,
					GalBDS3Sig:    0x11,
					GPSGLOBDS2Sig: 0x51,
				},
				VSolStatus:   SolComputed,
				VelType:      PosVelDopplerVelocity,
				VLatency:     0.0,
				VDiffAge:     0.0,
				HorSpd:       0.0053820245120602735,
				TrkGnd:       227.2379052688195,
				VertSpd:      -0.0011045472449647026,
				VertSpdSigma: 0.018328266218304634,
				HorSpdSigma:  0.013381028547883034,
			},
		},
		fixupMsgForAscii: func(msg *Msg) *Msg {
			newMsg := *msg
			newMsg.Body = fixupBestNavForAscii(msg.Body.(*BestNav))
			return &newMsg
		},
	},
}

var bestNavXYZTests = []dataTestCase{
	{
		name:      "BESTNAVXYZ with SINGLE fix",
		binPacket: mustHexDecode("aa44b561f000700000a06609c06787058c440000001216000000000010000000a490be42797731c1fce223cc983b57412359faaaabf336412918aa3f71b70e403ddbbe3f000000000800000068a42ea4befc11bf2d9b876e9eba73bfce6c5fe163065fbfc898153c79ffa03cdaab4e3c00000000000000000000000000000000331c1c000002115135185638"),
		asciiPacket: "#BESTNAVXYZA,97,GPS,FINE,2406,92760000,17548,0,18,22;SOL_COMPUTED,SINGLE,-1144697.2607,6090339.1897,1504171.6679,1.3289,2.2299,1.4911,SOL_COMPUTED,DOPPLER_VELOCITY,-0.0001,-0.0048,-0.0019,0.0091,0.0197,0.0126,\"\",0.000,0.000,0.000,51,28,28,0,0,02,11,51*85eccb2a\r\n",
		msg: &Msg{
			Hdr: MsgHdr{
				CPUIdlePercent: 97,
				TimingHdr: TimingHdr{
					TimeRef:            TimeRefGPS,
					TimeStatus:         TimeStatusFine,
					Week:               2406,
					MillisecondsOfWeek: 92760000,
					Reserved:           17548,
					Version:            0,
					LeapSec:            18,
					DelayMs:            22,
				},
			},
			Body: &BestNavXYZ{XYZ: novmsg.XYZ[SolStatus, PosVelType]{
				PSolStatus:    SolComputed,
				PosType:       PosVelSingle,
				PX:            -1144697.2607202912,
				PY:            6090339.189690348,
				PZ:            1504171.6678825102,
				PXSigma:       1.3288623094558716,
				PYSigma:       2.2299463748931885,
				PZSigma:       1.4910656213760376,
				VSolStatus:    SolComputed,
				VelType:       PosVelDopplerVelocity,
				VX:            -6.861604292275755e-05,
				VY:            -0.004816645502137712,
				VZ:            -0.001893613376060799,
				VXSigma:       0.00913066416978836,
				VYSigma:       0.019653068855404854,
				VZSigma:       0.012614214792847633,
				StnID:         StationID{},
				VLatency:      0.0,
				DiffAge:       0.0,
				SolAge:        0.0,
				NumSVs:        51,
				NumSolnSVs:    28,
				NumSolnL1SVs:  28,
				NumSolnMulti:  0,
				Reserved:      0,
				ExtSolStat:    0x02,
				GalBDS3Sig:    0x11,
				GPSGLOBDS2Sig: 0x51,
			}},
		},
		fixupMsgForAscii: func(msg *Msg) *Msg {
			newMsg := *msg
			newMsg.Body = fixupBestNavXYZForAscii(msg.Body.(*BestNavXYZ))
			return &newMsg
		},
	},
}

var pppNavTests = []dataTestCase{
	{
		name:        "PPPNAV with PPP fix",
		binPacket:   mustHexDecode("aa44b55e0204480000a06809e0e150068c440000001211000000000045000000b5cd4e6cb3762b4027e8115f432959400000e00b334d1340bba5f6c13d000000dd36f13cfb6b123d3245693d3939303100008040000000003722221e010087fb15e107f8"),
		asciiPacket: "#PPPNAVA,94,GPS,FINE,2408,105964000,17548,0,18,17;SOL_COMPUTED,PPP,13.73183763945,100.64473702191,4.8254,-30.8309,WGS84,0.0294,0.0357,0.0570,\"9901\",4.000,0.000,55,34,34,30,1,00,87,fb*f95d8344\r\n",
		msg: &Msg{
			Hdr: MsgHdr{
				CPUIdlePercent: 94,
				TimingHdr: TimingHdr{
					TimeRef:            TimeRefGPS,
					TimeStatus:         TimeStatusFine,
					Week:               2408,
					MillisecondsOfWeek: 105964000,
					Reserved:           17548,
					Version:            0,
					LeapSec:            18,
					DelayMs:            17,
				},
			},
			Body: &PPPNav{
				Pos: novmsg.Pos[SolStatus, PosVelType]{
					PSolStatus:    SolComputed,
					PosType:       PosVelPPP,
					Lat:           13.731837639445851,
					Lon:           100.64473702191081,
					Hgt:           4.825390039011836,
					Undulation:    -30.830923,
					DatumID:       DatumWGS84,
					LatSigma:      0.029445106,
					LonSigma:      0.03574751,
					HgtSigma:      0.056950755,
					StnID:         StationID{'9', '9', '0', '1'},
					DiffAge:       4,
					SolAge:        0,
					NumSVs:        55,
					NumSolnSVs:    34,
					NumSolnL1SVs:  34,
					NumSolnMulti:  30,
					Reserved:      1,
					ExtSolStat:    0x00,
					GalBDS3Sig:    0x87,
					GPSGLOBDS2Sig: 0xfb,
				},
			},
		},
		fixupMsgForAscii: func(msg *Msg) *Msg {
			newMsg := *msg
			newMsg.Body = fixupPPPNavForAscii(msg.Body.(*PPPNav))
			return &newMsg
		},
	},
}

func TestPPPNav(t *testing.T) {
	t.Run("bin", func(t *testing.T) {
		testDataBin(t, pppNavTests)
	})
	t.Run("ascii", func(t *testing.T) {
		testDataAscii(t, pppNavTests)
	})
}

func fixupPPPNavForAscii(msg MsgBody) MsgBody {
	m := msg.(*PPPNav)
	r := *m
	fixupFloat(&r.Lat, "%.11f")
	fixupFloat(&r.Lon, "%.11f")
	fixupFloat(&r.Hgt, "%.4f")
	fixupFloat32(&r.Undulation, "%.4f")
	fixupFloat32(&r.LatSigma, "%.4f")
	fixupFloat32(&r.LonSigma, "%.4f")
	fixupFloat32(&r.HgtSigma, "%.4f")
	return &r
}

func TestBestNav(t *testing.T) {
	t.Run("bin", func(t *testing.T) {
		testDataBin(t, bestNavTests)
	})
	t.Run("ascii", func(t *testing.T) {
		testDataAscii(t, bestNavTests)
	})
}

func TestBestNavXYZ(t *testing.T) {
	t.Run("bin", func(t *testing.T) {
		testDataBin(t, bestNavXYZTests)
	})
	t.Run("ascii", func(t *testing.T) {
		testDataAscii(t, bestNavXYZTests)
	})
}

func fixupBestNavForAscii(msg MsgBody) MsgBody {
	m := msg.(*BestNav)
	r := *m
	fixupFloat(&r.Lat, "%.11f")
	fixupFloat(&r.Lon, "%.11f")
	fixupFloat(&r.Hgt, "%.4f")
	fixupFloat32(&r.Undulation, "%.4f")
	fixupFloat32(&r.LatSigma, "%.4f")
	fixupFloat32(&r.LonSigma, "%.4f")
	fixupFloat32(&r.HgtSigma, "%.4f")
	fixupFloat(&r.HorSpd, "%.4f")
	fixupFloat(&r.TrkGnd, "%.6f")
	fixupFloat(&r.VertSpd, "%.4f")
	fixupFloat32(&r.VertSpdSigma, "%.4f")
	fixupFloat32(&r.HorSpdSigma, "%.4f")
	return &r
}

func fixupBestNavXYZForAscii(msg MsgBody) MsgBody {
	m := msg.(*BestNavXYZ)
	r := *m
	fixupFloat(&r.PX, "%.4f")
	fixupFloat(&r.PY, "%.4f")
	fixupFloat(&r.PZ, "%.4f")
	fixupFloat32(&r.PXSigma, "%.4f")
	fixupFloat32(&r.PYSigma, "%.4f")
	fixupFloat32(&r.PZSigma, "%.4f")
	fixupFloat(&r.VX, "%.4f")
	fixupFloat(&r.VY, "%.4f")
	fixupFloat(&r.VZ, "%.4f")
	fixupFloat32(&r.VXSigma, "%.4f")
	fixupFloat32(&r.VYSigma, "%.4f")
	fixupFloat32(&r.VZSigma, "%.4f")
	return &r
}

// fixupFloat32 simulates the receiver's float formatting for float32 values
func fixupFloat32(val *float32, format string) {
	str := fmt.Sprintf(format, *val)
	result, err := strconv.ParseFloat(str, 32)
	if err != nil {
		panic(fmt.Sprintf("failed to round-trip float32 %v: %v", *val, err))
	}
	*val = float32(result)
}
