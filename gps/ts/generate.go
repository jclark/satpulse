// Package ts provides TypeScript type definitions for GPS JSON serialisation.
// See README.md for details.
//
//go:generate go run gen.go
package ts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/ascii"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/ptime"
)

type sample struct {
	tsType string
	value  any
}

type configTargetVocabulary struct {
	Props []gpsprot.ConfigProps
	Save  []gpsprot.SaveType
	Reset []gpsprot.ResetType
	Get   []gpsprot.PropIDs
}

func samples() []sample {
	speed := 9600
	return []sample{
		{"SatellitesMsg", gpsprot.SatellitesMsg{
			Tag:         "UBX",
			NativeMsgID: "NAV-SAT",
			SVs: []gpsprot.SVInfo{
				{
					ID:         gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1},
					LookAngles: opt.Make(gpsprot.LookAngles{Azimuth: 45, Elevation: 30}),
					Signals:    []gpsprot.SignalInfo{{ID: "L1 C/A", CN0: 42, Used: true}},
					Used:       true,
				},
			},
			UsedValidity: gpsprot.SatelliteUsedSignal,
		}},
		{"TimeMsg", gpsprot.TimeMsg{
			TAITime:     ptime.Time(1741320000 * 1e9),
			UTCTime:     utcTime(2025, 3, 7, 4, 0, 0),
			Accuracy:    25 * time.Millisecond,
			UTCOffset:   37,
			PulseOffset: opt.Make(0.000000125),
			GNSS:        gpsprot.GPS,
			Ref:         gpsprot.NavSolution,
			ReadDelay:   15 * gpsprot.Millisecond,
			Tag:         "UBX",
			NativeMsgID: "NAV-TIMEGPS",
		}},
		{"SurveyMsg", gpsprot.SurveyMsg{
			Position:   gpsprot.Point3D{4075539814000, 555132076000, 4828427312000},
			Accuracy:   25000,
			ObsCount:   3600,
			ObsTime:    gpsprot.Duration(3600 * time.Second),
			Valid:      true,
			InProgress: false,
		}},
		{"PosGeoMsg", gpsprot.PosGeoMsg{
			LatLon:      [2]gpsprot.Angle{gpsprot.DegreesFromFloat(51.477928), gpsprot.DegreesFromFloat(-0.001545)},
			Height:      opt.Make(gpsprot.Length(45321000)),
			HeightMSL:   opt.Make(gpsprot.Length(0)),
			Tag:         "UBX",
			NativeMsgID: "NAV-PVT",
		}},
		{"PosECEFMsg", gpsprot.PosECEFMsg{
			Pos:         gpsprot.Point3D{4075539814000, 555132076000, 4828427312000},
			Tag:         "UBX",
			NativeMsgID: "NAV-PVT",
		}},
		{"VelGeoMsg", gpsprot.VelGeoMsg{
			VelNED:      opt.Make([3]gpsprot.Speed{15000, -3000, 1000}),
			GroundSpeed: opt.Make(gpsprot.Speed(15000)),
			Speed3D:     opt.Make(gpsprot.Speed(16000)),
			Course:      opt.Make(gpsprot.DegreesFromFloat(78.5)),
			Tag:         "UBX",
			NativeMsgID: "NAV-PVT",
		}},
		{"VelECEFMsg", gpsprot.VelECEFMsg{
			Vel:         [3]gpsprot.Speed{12000, -5000, 3000},
			Tag:         "UBX",
			NativeMsgID: "NAV-PVT",
		}},
		{"PVMsgBundle", gpsprot.PVMsgBundle{
			PosGeo: opt.Make(gpsprot.PosGeoMsg{
				LatLon:      [2]gpsprot.Angle{gpsprot.DegreesFromFloat(51.477928), gpsprot.DegreesFromFloat(-0.001545)},
				Tag:         "UBX",
				NativeMsgID: "NAV-PVT",
			}),
			VelECEF: opt.Make(gpsprot.VelECEFMsg{
				Vel:         [3]gpsprot.Speed{12000, -5000, 3000},
				Tag:         "UBX",
				NativeMsgID: "NAV-PVT",
			}),
		}},
		{"NavEpochMsg", gpsprot.NavEpochMsg{
			FixLevel:    gpsprot.FixLevelCarrierFixed,
			SolutionDim: gpsprot.SolutionDim3D,
			Correction:  gpsprot.CorrRTCM,
			Acc:         gpsprot.Accuracy{Hor: opt.Make(gpsprot.Length(15000)), Vert: opt.Make(gpsprot.Length(25000))},
			DOP:         gpsprot.DOP{Pos: opt.Make(1.2), Hor: opt.Make(0.8), North: opt.Make(0.6), East: opt.Make(0.5)},
			NumSVUsed:   opt.Make[uint16](12),
			GNSSUsed:    gpsprot.GNSSSetOf(gpsprot.GPS) | gpsprot.GNSSSetOf(gpsprot.GAL),
			BandsUsed:   gpsprot.BandL1 | gpsprot.BandL5,
			Tag:         "UBX",
		}},
		{"CorReportMsg", gpsprot.CorReportMsg{
			Source:        gpsprot.CorReportSourcePull,
			Tag:           "RTCM",
			MsgID:         "1077",
			NBytes:        opt.Make(42),
			ChecksumOK:    opt.Make(true),
			FinalFragment: opt.Make(true),
			Used:          opt.Make(false),
			RTCMRefBaseID: opt.Make(uint16(123)),
		}},
		{"LeapSecondMsg", gpsprot.LeapSecondMsg{
			LeapSecond: ptime.LeapSecond{
				OffChangeTime: ptime.Time(1735689637 * 1e9),
				UTCOffBefore:  37,
				UTCOffAfter:   37,
			},
			GNSS: gpsprot.GPS,
		}},
		{"ReceiverInfo", gpsprot.ReceiverInfo{
			Vendor:        "u-blox",
			Firmware:      "TIM 2.20 PROTVER 18.00",
			Hardware:      "ZED-F9T",
			SupportedGNSS: 1<<gpsprot.GPS | 1<<gpsprot.GAL | 1<<gpsprot.BDS | 1<<gpsprot.GLO,
		}},
		{"PacketLogEntry", gpsio.PacketLogEntry{
			T:     gpsio.TimeMicro(time.Date(2025, 3, 7, 4, 0, 0, 123456000, time.UTC)),
			Tag:   "UBX",
			Msg:   "NAV-PVT",
			Bin:   gpsio.HexString{0xb5, 0x62, 0x01, 0x07},
			Speed: &speed,
			Out:   true,
		}},
		{"ConfigTarget", configTargetSample()},
		{"ConfigTargetVocabulary", configTargetVocabularySample()},
	}
}

func configTargetSample() *gpsprot.ConfigTarget {
	cp := configPropsSamples()[0]
	cp.ClearReadOnlyProps()
	var root [32]byte
	for i := range root {
		root[i] = byte(i)
	}
	return &gpsprot.ConfigTarget{
		Props: cp,
		Get:   gpsprot.PropIDSignalsEnabled | gpsprot.PropIDTimePulse | gpsprot.PropIDPort,
		Opts: gpsprot.ConfigOptions{
			Socket:     true,
			ForceProbe: true,
			Save:       gpsprot.SaveMinimal,
			Reset:      gpsprot.ResetCold,
			PVTMsg:     gpsprot.PVTMsgPos | gpsprot.PVTMsgVel | gpsprot.PVTMsgTime | gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgLeapSecond | gpsprot.PVTMsgSurvey | gpsprot.PVTMsgTAI | gpsprot.PVTMsgECEF | gpsprot.PVTMsgTimePulseAfter | gpsprot.PVTMsgQuality | gpsprot.PVTMsgEpoch | gpsprot.PVTMsgOff,
			NMEAMsg:    opt.Make(gpsprot.NMEAMsgAny),
			RTCMMsg:    opt.Make(gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgMSM7 | gpsprot.RTCMMsgARP | gpsprot.RTCMMsgLax | gpsprot.RTCMMsgOther),
			SatsMsg:    opt.Make(gpsprot.SatsMsgAny),
			RawMsg:     opt.Make(gpsprot.RawMsgAny),
			Survey: gpsprot.Survey{
				Flags:    gpsprot.SurveyAgain,
				MinDur:   30 * time.Minute,
				AccLimit: gpsprot.Meters(1.5),
			},
			SetStatic: true,
			TimeAssist: gpsprot.TimeEstimate{
				EstimatedTime:  time.Date(2025, 3, 7, 4, 0, 0, 0, time.UTC),
				TimeOfEstimate: time.Date(2025, 3, 7, 4, 0, 1, 0, time.UTC),
				Accuracy:       500 * time.Millisecond,
				LeapSecond: ptime.LeapSecondState{
					UTCOffset:   37,
					LeapTonight: ptime.LeapSecondPositive,
				},
				Trusted: true,
			},
			OSNMA: gpsprot.OSNMAOptions{MerkleTreeRoot: root},
		},
	}
}

func configTargetVocabularySample() configTargetVocabulary {
	return configTargetVocabulary{
		Props: configPropsSamples(),
		Save:  []gpsprot.SaveType{gpsprot.SaveNone, gpsprot.SaveMinimal, gpsprot.SaveAll},
		Reset: []gpsprot.ResetType{gpsprot.ResetNone, gpsprot.ResetReload, gpsprot.ResetCold, gpsprot.ResetFactory},
		Get: []gpsprot.PropIDs{
			gpsprot.PropIDSignalsEnabled,
			gpsprot.PropIDTimeGNSS,
			gpsprot.PropIDTimePulse,
			gpsprot.PropIDTimePulseWidth,
			gpsprot.PropIDTimePulsePeriod,
			gpsprot.PropIDTimePulseAlignToGNSS,
			gpsprot.PropIDTimePulseOnlyWhenLocked,
			gpsprot.PropIDTimePulsePolarityRising,
			gpsprot.PropIDMode,
			gpsprot.PropIDAntennaCableDelay,
			gpsprot.PropIDNavMsgAuth,
			gpsprot.PropIDRTCMBaseID,
			gpsprot.PropIDMinElevation,
			gpsprot.PropIDBaudRate,
			gpsprot.PropIDPort,
		},
	}
}

func configPropsSamples() []gpsprot.ConfigProps {
	var ecef gpsprot.ConfigProps
	ecef.SetSignalsEnabled(gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGALE1))
	ecef.SetTimeGNSS(gpsprot.GPS)
	ecef.SetTimePulse(gpsprot.TimePulse{
		Width:          100 * time.Millisecond,
		Period:         time.Second,
		AlignToGNSS:    true,
		OnlyWhenLocked: true,
		PolarityRising: true,
	})
	ecef.SetMode(gpsprot.Mode{
		Static:       true,
		PosType:      gpsprot.PosTypeECEF,
		FixedPosECEF: gpsprot.Point3D{gpsprot.Meters(4075539.814), gpsprot.Meters(555132.076), gpsprot.Meters(4828427.312)},
		FixedPosAcc:  gpsprot.Meters(0.025),
	})
	ecef.SetAntennaCableDelay(50 * time.Nanosecond)
	ecef.SetNavMsgAuth(gpsprot.NavMsgAuthOSNMA)
	ecef.SetRTCMBaseID(42)
	ecef.SetMinElevation(gpsprot.DegreesFromFloat(10))
	ecef.SetBaudRate(115200)
	ecef.SetPort("UART1")
	var llh gpsprot.ConfigProps
	llh.SetTimePulseWidth(200 * time.Millisecond)
	llh.SetMode(gpsprot.Mode{
		Static:      true,
		PosType:     gpsprot.PosTypeLLH,
		FixedPosLLH: [2]gpsprot.Angle{gpsprot.DegreesFromFloat(13.75), gpsprot.DegreesFromFloat(100.5)},
		Height:      gpsprot.Meters(12.5),
		FixedPosAcc: gpsprot.Meters(0.05),
	})
	llh.SetNavMsgAuth(gpsprot.NavMsgAuthNone)
	return []gpsprot.ConfigProps{ecef, llh}
}

// Generate produces the contents of validate.gen.ts.
func Generate() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("// Code generated by go generate; DO NOT EDIT.\n")
	buf.WriteString("import type {")
	gpsprots := []string{
		"SatellitesMsg", "TimeMsg", "SurveyMsg",
		"PosGeoMsg", "PosECEFMsg", "VelGeoMsg", "VelECEFMsg", "PVMsgBundle",
		"NavEpochMsg", "CorReportMsg", "LeapSecondMsg", "ReceiverInfo",
	}
	for i, name := range gpsprots {
		if i > 0 {
			buf.WriteString(",")
		}
		buf.WriteString(" " + name)
	}
	buf.WriteString("} from './gpsprot';\n")
	buf.WriteString("import type {PacketLogEntry} from './gpsio';\n")
	buf.WriteString("import type {ConfigTarget, ConfigTargetVocabulary} from './configtarget';\n")
	buf.WriteString("\n")
	for _, s := range samples() {
		j, err := json.Marshal(s.value)
		if err != nil {
			return nil, fmt.Errorf("marshal %s: %w", s.tsType, err)
		}
		fmt.Fprintf(&buf, "const _%s: %s = %s;\n", lcFirst(s.tsType), s.tsType, j)
	}
	return buf.Bytes(), nil
}

func lcFirst(s string) string {
	if s == "" {
		return s
	}
	return string(ascii.ToLower(s[0])) + s[1:]
}

func utcTime(year int, month time.Month, day, hour, min, sec int) opt.Val[ptime.UTCTime] {
	date := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	tod := time.Duration(hour)*time.Hour + time.Duration(min)*time.Minute + time.Duration(sec)*time.Second
	return opt.Make(ptime.UTCTime{Date: date, TimeOfDay: tod})
}
