package uncmsg

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"strconv"
	"testing"
)

var dataTests = []struct {
	name        string
	binPacket   []byte
	asciiPacket string
	header      MessageHeader
	value       Msg
	// fixupValueForBin transforms value from ASCII message into value expected by binary format
	// This is used for VERSIONA/VERSIONB inconsistency:
	// VERSIONB has just build number; VERSIONA has additional info
	fixupValueForBin func(Msg) Msg // Transform ASCII baseline to binary format
	// fixupValueForAscii transforms value from binary message into value expected by ASCII format
	// This is used to handle floating point differences, where ASCII has limited precision.
	fixupValueForAscii func(Msg) Msg // Transform binary baseline to ASCII format
}{
	{
		name:        "SatsInfo with 47 satellites",
		binPacket:   mustHexDecode("aa44b55e4c08f60200a04909a058d9000000000000121c002f020000003f18af00130020000400251104002a0e04002409041d3c0133002c000300281103002c090319ea0013002800040025110400240e04002809040cc900140028000300211103002609030fe0004f003100030030110300310903170d0112002000050020110500240e0500210305001d09050d41003c001c0001c2950013052600040525110405260e0405280304c774003805250004052c1104052d0e0405270304c4360035051a1103051b0e0305190303c3490031051a0e013b75001701220002011d07022d150017011a00020116050227e2002a012e0002012b05023c460033011a00013d5c011c011b0003011a05030120070326440146012b00013cf2003e04320003042e150304310d0302e90042042b000304321103042c1503038c004704300003042e1103042b150305ff0027042a0003042b1103042a15030d4e012c04240003042411030422150325c0000804240005042415050421080504260c0504280d0514e6000d04290005042515050426080504230c0504250d052791001c042a0005042615050428080504250c0504260d0506a0001804240003042c11030428150309ad001304220003042711030422150328a00037042b0005042e1505042a0805042a0c05042d0d0507b0003c042d0003042d1103042c15031c6d0036042b0005042715050429080504260c0504290d050aca00450430000304301103042e1503109e0017042800030428110304221503085b012a041900030420110304191503200f013e04300005042f1505042f080504300c0504320d051b03004104270005042615050427080504240c0504260d05242b0029031902011e19010f031d02040325110403220c0403201604092b001d03181101056c004a033002040334110403310c04033016042252011e031a11020317160219b0000d031c020303180c030317160303c80025032c0204032f1104032b0c040329160402e4001b032c0204032d1104032b0c04032a160428750034062c060180f40022063006013e04010c06260601ba050126062e0601b62e3613"),
		asciiPacket: "#SATSINFOA,94,GPS,FINE,2377,14244000,0,0,18,28;47,2,0,0,0,63,24,175,19,0,32,0,4,0,37,17,4,0,42,14,4,0,36,9,4,29,316,51,0,44,0,3,0,40,17,3,0,44,9,3,25,234,19,0,40,0,4,0,37,17,4,0,36,14,4,0,40,9,4,12,201,20,0,40,0,3,0,33,17,3,0,38,9,3,15,224,79,0,49,0,3,0,48,17,3,0,49,9,3,23,269,18,0,32,0,5,0,32,17,5,0,36,14,5,0,33,3,5,0,29,9,5,13,65,60,0,28,0,1,194,149,19,5,38,0,4,5,37,17,4,5,38,14,4,5,40,3,4,199,116,56,5,37,0,4,5,44,17,4,5,45,14,4,5,39,3,4,196,54,53,5,26,17,3,5,27,14,3,5,25,3,3,195,73,49,5,26,14,1,59,117,23,1,34,0,2,1,29,7,2,45,21,23,1,26,0,2,1,22,5,2,39,226,42,1,46,0,2,1,43,5,2,60,70,51,1,26,0,1,61,348,28,1,27,0,3,1,26,5,3,1,32,7,3,38,324,70,1,43,0,1,60,242,62,4,50,0,3,4,46,21,3,4,49,13,3,2,233,66,4,43,0,3,4,50,17,3,4,44,21,3,3,140,71,4,48,0,3,4,46,17,3,4,43,21,3,5,255,39,4,42,0,3,4,43,17,3,4,42,21,3,13,334,44,4,36,0,3,4,36,17,3,4,34,21,3,37,192,8,4,36,0,5,4,36,21,5,4,33,8,5,4,38,12,5,4,40,13,5,20,230,13,4,41,0,5,4,37,21,5,4,38,8,5,4,35,12,5,4,37,13,5,39,145,28,4,42,0,5,4,38,21,5,4,40,8,5,4,37,12,5,4,38,13,5,6,160,24,4,36,0,3,4,44,17,3,4,40,21,3,9,173,19,4,34,0,3,4,39,17,3,4,34,21,3,40,160,55,4,43,0,5,4,46,21,5,4,42,8,5,4,42,12,5,4,45,13,5,7,176,60,4,45,0,3,4,45,17,3,4,44,21,3,28,109,54,4,43,0,5,4,39,21,5,4,41,8,5,4,38,12,5,4,41,13,5,10,202,69,4,48,0,3,4,48,17,3,4,46,21,3,16,158,23,4,40,0,3,4,40,17,3,4,34,21,3,8,347,42,4,25,0,3,4,32,17,3,4,25,21,3,32,271,62,4,48,0,5,4,47,21,5,4,47,8,5,4,48,12,5,4,50,13,5,27,3,65,4,39,0,5,4,38,21,5,4,39,8,5,4,36,12,5,4,38,13,5,36,43,41,3,25,2,1,30,281,15,3,29,2,4,3,37,17,4,3,34,12,4,3,32,22,4,9,43,29,3,24,17,1,5,108,74,3,48,2,4,3,52,17,4,3,49,12,4,3,48,22,4,34,338,30,3,26,17,2,3,23,22,2,25,176,13,3,28,2,3,3,24,12,3,3,23,22,3,3,200,37,3,44,2,4,3,47,17,4,3,43,12,4,3,41,22,4,2,228,27,3,44,2,4,3,45,17,4,3,43,12,4,3,42,22,4,40,117,52,6,44,6,1,128,244,34,6,48,6,1,62,260,12,6,38,6,1,186,261,38,6,46,6,1*5f93d72e\r\n",
		header: MessageHeader{
			CPUIdlePercent: 94,
			TimingHeader: TimingHeader{
				TimeRef:            TimeRefGPS,
				TimeStatus:         TimeStatusFine,
				Week:               2377,
				MillisecondsOfWeek: 14244000,
				Reserved:           0,
				Version:            0,
				LeapSec:            18,
				DelayMs:            28,
			},
		},
		value: &SatsInfo{
			SatsInfoInitChunk: SatsInfoInitChunk{
				SatNumber:     47,
				VersionNumber: 2,
				FreqFlag:      63,
			},
			Sats: []SatsInfoSat{
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 24, Azimuth: 175, Elevation: 19}, Freqs: []SatsInfoFreq{{SysStatus: 0, SNR: 32, FreqStatus: 0, FreqNo: 4}, {SysStatus: 0, SNR: 37, FreqStatus: 17, FreqNo: 4}, {SysStatus: 0, SNR: 42, FreqStatus: 14, FreqNo: 4}, {SysStatus: 0, SNR: 36, FreqStatus: 9, FreqNo: 4}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 29, Azimuth: 316, Elevation: 51}, Freqs: []SatsInfoFreq{{SysStatus: 0, SNR: 44, FreqStatus: 0, FreqNo: 3}, {SysStatus: 0, SNR: 40, FreqStatus: 17, FreqNo: 3}, {SysStatus: 0, SNR: 44, FreqStatus: 9, FreqNo: 3}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 25, Azimuth: 234, Elevation: 19}, Freqs: []SatsInfoFreq{{SysStatus: 0, SNR: 40, FreqStatus: 0, FreqNo: 4}, {SysStatus: 0, SNR: 37, FreqStatus: 17, FreqNo: 4}, {SysStatus: 0, SNR: 36, FreqStatus: 14, FreqNo: 4}, {SysStatus: 0, SNR: 40, FreqStatus: 9, FreqNo: 4}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 12, Azimuth: 201, Elevation: 20}, Freqs: []SatsInfoFreq{{SysStatus: 0, SNR: 40, FreqStatus: 0, FreqNo: 3}, {SysStatus: 0, SNR: 33, FreqStatus: 17, FreqNo: 3}, {SysStatus: 0, SNR: 38, FreqStatus: 9, FreqNo: 3}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 15, Azimuth: 224, Elevation: 79}, Freqs: []SatsInfoFreq{{SysStatus: 0, SNR: 49, FreqStatus: 0, FreqNo: 3}, {SysStatus: 0, SNR: 48, FreqStatus: 17, FreqNo: 3}, {SysStatus: 0, SNR: 49, FreqStatus: 9, FreqNo: 3}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 23, Azimuth: 269, Elevation: 18}, Freqs: []SatsInfoFreq{{SysStatus: 0, SNR: 32, FreqStatus: 0, FreqNo: 5}, {SysStatus: 0, SNR: 32, FreqStatus: 17, FreqNo: 5}, {SysStatus: 0, SNR: 36, FreqStatus: 14, FreqNo: 5}, {SysStatus: 0, SNR: 33, FreqStatus: 3, FreqNo: 5}, {SysStatus: 0, SNR: 29, FreqStatus: 9, FreqNo: 5}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 13, Azimuth: 65, Elevation: 60}, Freqs: []SatsInfoFreq{{SysStatus: 0, SNR: 28, FreqStatus: 0, FreqNo: 1}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 194, Azimuth: 149, Elevation: 19}, Freqs: []SatsInfoFreq{{SysStatus: 5, SNR: 38, FreqStatus: 0, FreqNo: 4}, {SysStatus: 5, SNR: 37, FreqStatus: 17, FreqNo: 4}, {SysStatus: 5, SNR: 38, FreqStatus: 14, FreqNo: 4}, {SysStatus: 5, SNR: 40, FreqStatus: 3, FreqNo: 4}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 199, Azimuth: 116, Elevation: 56}, Freqs: []SatsInfoFreq{{SysStatus: 5, SNR: 37, FreqStatus: 0, FreqNo: 4}, {SysStatus: 5, SNR: 44, FreqStatus: 17, FreqNo: 4}, {SysStatus: 5, SNR: 45, FreqStatus: 14, FreqNo: 4}, {SysStatus: 5, SNR: 39, FreqStatus: 3, FreqNo: 4}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 196, Azimuth: 54, Elevation: 53}, Freqs: []SatsInfoFreq{{SysStatus: 5, SNR: 26, FreqStatus: 17, FreqNo: 3}, {SysStatus: 5, SNR: 27, FreqStatus: 14, FreqNo: 3}, {SysStatus: 5, SNR: 25, FreqStatus: 3, FreqNo: 3}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 195, Azimuth: 73, Elevation: 49}, Freqs: []SatsInfoFreq{{SysStatus: 5, SNR: 26, FreqStatus: 14, FreqNo: 1}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 59, Azimuth: 117, Elevation: 23}, Freqs: []SatsInfoFreq{{SysStatus: 1, SNR: 34, FreqStatus: 0, FreqNo: 2}, {SysStatus: 1, SNR: 29, FreqStatus: 7, FreqNo: 2}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 45, Azimuth: 21, Elevation: 23}, Freqs: []SatsInfoFreq{{SysStatus: 1, SNR: 26, FreqStatus: 0, FreqNo: 2}, {SysStatus: 1, SNR: 22, FreqStatus: 5, FreqNo: 2}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 39, Azimuth: 226, Elevation: 42}, Freqs: []SatsInfoFreq{{SysStatus: 1, SNR: 46, FreqStatus: 0, FreqNo: 2}, {SysStatus: 1, SNR: 43, FreqStatus: 5, FreqNo: 2}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 60, Azimuth: 70, Elevation: 51}, Freqs: []SatsInfoFreq{{SysStatus: 1, SNR: 26, FreqStatus: 0, FreqNo: 1}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 61, Azimuth: 348, Elevation: 28}, Freqs: []SatsInfoFreq{{SysStatus: 1, SNR: 27, FreqStatus: 0, FreqNo: 3}, {SysStatus: 1, SNR: 26, FreqStatus: 5, FreqNo: 3}, {SysStatus: 1, SNR: 32, FreqStatus: 7, FreqNo: 3}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 38, Azimuth: 324, Elevation: 70}, Freqs: []SatsInfoFreq{{SysStatus: 1, SNR: 43, FreqStatus: 0, FreqNo: 1}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 60, Azimuth: 242, Elevation: 62}, Freqs: []SatsInfoFreq{{SysStatus: 4, SNR: 50, FreqStatus: 0, FreqNo: 3}, {SysStatus: 4, SNR: 46, FreqStatus: 21, FreqNo: 3}, {SysStatus: 4, SNR: 49, FreqStatus: 13, FreqNo: 3}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 2, Azimuth: 233, Elevation: 66}, Freqs: []SatsInfoFreq{{SysStatus: 4, SNR: 43, FreqStatus: 0, FreqNo: 3}, {SysStatus: 4, SNR: 50, FreqStatus: 17, FreqNo: 3}, {SysStatus: 4, SNR: 44, FreqStatus: 21, FreqNo: 3}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 3, Azimuth: 140, Elevation: 71}, Freqs: []SatsInfoFreq{{SysStatus: 4, SNR: 48, FreqStatus: 0, FreqNo: 3}, {SysStatus: 4, SNR: 46, FreqStatus: 17, FreqNo: 3}, {SysStatus: 4, SNR: 43, FreqStatus: 21, FreqNo: 3}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 5, Azimuth: 255, Elevation: 39}, Freqs: []SatsInfoFreq{{SysStatus: 4, SNR: 42, FreqStatus: 0, FreqNo: 3}, {SysStatus: 4, SNR: 43, FreqStatus: 17, FreqNo: 3}, {SysStatus: 4, SNR: 42, FreqStatus: 21, FreqNo: 3}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 13, Azimuth: 334, Elevation: 44}, Freqs: []SatsInfoFreq{{SysStatus: 4, SNR: 36, FreqStatus: 0, FreqNo: 3}, {SysStatus: 4, SNR: 36, FreqStatus: 17, FreqNo: 3}, {SysStatus: 4, SNR: 34, FreqStatus: 21, FreqNo: 3}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 37, Azimuth: 192, Elevation: 8}, Freqs: []SatsInfoFreq{{SysStatus: 4, SNR: 36, FreqStatus: 0, FreqNo: 5}, {SysStatus: 4, SNR: 36, FreqStatus: 21, FreqNo: 5}, {SysStatus: 4, SNR: 33, FreqStatus: 8, FreqNo: 5}, {SysStatus: 4, SNR: 38, FreqStatus: 12, FreqNo: 5}, {SysStatus: 4, SNR: 40, FreqStatus: 13, FreqNo: 5}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 20, Azimuth: 230, Elevation: 13}, Freqs: []SatsInfoFreq{{SysStatus: 4, SNR: 41, FreqStatus: 0, FreqNo: 5}, {SysStatus: 4, SNR: 37, FreqStatus: 21, FreqNo: 5}, {SysStatus: 4, SNR: 38, FreqStatus: 8, FreqNo: 5}, {SysStatus: 4, SNR: 35, FreqStatus: 12, FreqNo: 5}, {SysStatus: 4, SNR: 37, FreqStatus: 13, FreqNo: 5}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 39, Azimuth: 145, Elevation: 28}, Freqs: []SatsInfoFreq{{SysStatus: 4, SNR: 42, FreqStatus: 0, FreqNo: 5}, {SysStatus: 4, SNR: 38, FreqStatus: 21, FreqNo: 5}, {SysStatus: 4, SNR: 40, FreqStatus: 8, FreqNo: 5}, {SysStatus: 4, SNR: 37, FreqStatus: 12, FreqNo: 5}, {SysStatus: 4, SNR: 38, FreqStatus: 13, FreqNo: 5}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 6, Azimuth: 160, Elevation: 24}, Freqs: []SatsInfoFreq{{SysStatus: 4, SNR: 36, FreqStatus: 0, FreqNo: 3}, {SysStatus: 4, SNR: 44, FreqStatus: 17, FreqNo: 3}, {SysStatus: 4, SNR: 40, FreqStatus: 21, FreqNo: 3}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 9, Azimuth: 173, Elevation: 19}, Freqs: []SatsInfoFreq{{SysStatus: 4, SNR: 34, FreqStatus: 0, FreqNo: 3}, {SysStatus: 4, SNR: 39, FreqStatus: 17, FreqNo: 3}, {SysStatus: 4, SNR: 34, FreqStatus: 21, FreqNo: 3}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 40, Azimuth: 160, Elevation: 55}, Freqs: []SatsInfoFreq{{SysStatus: 4, SNR: 43, FreqStatus: 0, FreqNo: 5}, {SysStatus: 4, SNR: 46, FreqStatus: 21, FreqNo: 5}, {SysStatus: 4, SNR: 42, FreqStatus: 8, FreqNo: 5}, {SysStatus: 4, SNR: 42, FreqStatus: 12, FreqNo: 5}, {SysStatus: 4, SNR: 45, FreqStatus: 13, FreqNo: 5}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 7, Azimuth: 176, Elevation: 60}, Freqs: []SatsInfoFreq{{SysStatus: 4, SNR: 45, FreqStatus: 0, FreqNo: 3}, {SysStatus: 4, SNR: 45, FreqStatus: 17, FreqNo: 3}, {SysStatus: 4, SNR: 44, FreqStatus: 21, FreqNo: 3}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 28, Azimuth: 109, Elevation: 54}, Freqs: []SatsInfoFreq{{SysStatus: 4, SNR: 43, FreqStatus: 0, FreqNo: 5}, {SysStatus: 4, SNR: 39, FreqStatus: 21, FreqNo: 5}, {SysStatus: 4, SNR: 41, FreqStatus: 8, FreqNo: 5}, {SysStatus: 4, SNR: 38, FreqStatus: 12, FreqNo: 5}, {SysStatus: 4, SNR: 41, FreqStatus: 13, FreqNo: 5}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 10, Azimuth: 202, Elevation: 69}, Freqs: []SatsInfoFreq{{SysStatus: 4, SNR: 48, FreqStatus: 0, FreqNo: 3}, {SysStatus: 4, SNR: 48, FreqStatus: 17, FreqNo: 3}, {SysStatus: 4, SNR: 46, FreqStatus: 21, FreqNo: 3}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 16, Azimuth: 158, Elevation: 23}, Freqs: []SatsInfoFreq{{SysStatus: 4, SNR: 40, FreqStatus: 0, FreqNo: 3}, {SysStatus: 4, SNR: 40, FreqStatus: 17, FreqNo: 3}, {SysStatus: 4, SNR: 34, FreqStatus: 21, FreqNo: 3}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 8, Azimuth: 347, Elevation: 42}, Freqs: []SatsInfoFreq{{SysStatus: 4, SNR: 25, FreqStatus: 0, FreqNo: 3}, {SysStatus: 4, SNR: 32, FreqStatus: 17, FreqNo: 3}, {SysStatus: 4, SNR: 25, FreqStatus: 21, FreqNo: 3}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 32, Azimuth: 271, Elevation: 62}, Freqs: []SatsInfoFreq{{SysStatus: 4, SNR: 48, FreqStatus: 0, FreqNo: 5}, {SysStatus: 4, SNR: 47, FreqStatus: 21, FreqNo: 5}, {SysStatus: 4, SNR: 47, FreqStatus: 8, FreqNo: 5}, {SysStatus: 4, SNR: 48, FreqStatus: 12, FreqNo: 5}, {SysStatus: 4, SNR: 50, FreqStatus: 13, FreqNo: 5}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 27, Azimuth: 3, Elevation: 65}, Freqs: []SatsInfoFreq{{SysStatus: 4, SNR: 39, FreqStatus: 0, FreqNo: 5}, {SysStatus: 4, SNR: 38, FreqStatus: 21, FreqNo: 5}, {SysStatus: 4, SNR: 39, FreqStatus: 8, FreqNo: 5}, {SysStatus: 4, SNR: 36, FreqStatus: 12, FreqNo: 5}, {SysStatus: 4, SNR: 38, FreqStatus: 13, FreqNo: 5}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 36, Azimuth: 43, Elevation: 41}, Freqs: []SatsInfoFreq{{SysStatus: 3, SNR: 25, FreqStatus: 2, FreqNo: 1}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 30, Azimuth: 281, Elevation: 15}, Freqs: []SatsInfoFreq{{SysStatus: 3, SNR: 29, FreqStatus: 2, FreqNo: 4}, {SysStatus: 3, SNR: 37, FreqStatus: 17, FreqNo: 4}, {SysStatus: 3, SNR: 34, FreqStatus: 12, FreqNo: 4}, {SysStatus: 3, SNR: 32, FreqStatus: 22, FreqNo: 4}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 9, Azimuth: 43, Elevation: 29}, Freqs: []SatsInfoFreq{{SysStatus: 3, SNR: 24, FreqStatus: 17, FreqNo: 1}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 5, Azimuth: 108, Elevation: 74}, Freqs: []SatsInfoFreq{{SysStatus: 3, SNR: 48, FreqStatus: 2, FreqNo: 4}, {SysStatus: 3, SNR: 52, FreqStatus: 17, FreqNo: 4}, {SysStatus: 3, SNR: 49, FreqStatus: 12, FreqNo: 4}, {SysStatus: 3, SNR: 48, FreqStatus: 22, FreqNo: 4}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 34, Azimuth: 338, Elevation: 30}, Freqs: []SatsInfoFreq{{SysStatus: 3, SNR: 26, FreqStatus: 17, FreqNo: 2}, {SysStatus: 3, SNR: 23, FreqStatus: 22, FreqNo: 2}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 25, Azimuth: 176, Elevation: 13}, Freqs: []SatsInfoFreq{{SysStatus: 3, SNR: 28, FreqStatus: 2, FreqNo: 3}, {SysStatus: 3, SNR: 24, FreqStatus: 12, FreqNo: 3}, {SysStatus: 3, SNR: 23, FreqStatus: 22, FreqNo: 3}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 3, Azimuth: 200, Elevation: 37}, Freqs: []SatsInfoFreq{{SysStatus: 3, SNR: 44, FreqStatus: 2, FreqNo: 4}, {SysStatus: 3, SNR: 47, FreqStatus: 17, FreqNo: 4}, {SysStatus: 3, SNR: 43, FreqStatus: 12, FreqNo: 4}, {SysStatus: 3, SNR: 41, FreqStatus: 22, FreqNo: 4}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 2, Azimuth: 228, Elevation: 27}, Freqs: []SatsInfoFreq{{SysStatus: 3, SNR: 44, FreqStatus: 2, FreqNo: 4}, {SysStatus: 3, SNR: 45, FreqStatus: 17, FreqNo: 4}, {SysStatus: 3, SNR: 43, FreqStatus: 12, FreqNo: 4}, {SysStatus: 3, SNR: 42, FreqStatus: 22, FreqNo: 4}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 40, Azimuth: 117, Elevation: 52}, Freqs: []SatsInfoFreq{{SysStatus: 6, SNR: 44, FreqStatus: 6, FreqNo: 1}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 128, Azimuth: 244, Elevation: 34}, Freqs: []SatsInfoFreq{{SysStatus: 6, SNR: 48, FreqStatus: 6, FreqNo: 1}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 62, Azimuth: 260, Elevation: 12}, Freqs: []SatsInfoFreq{{SysStatus: 6, SNR: 38, FreqStatus: 6, FreqNo: 1}}},
				{SatsInfoSatChunk: SatsInfoSatChunk{PRN: 186, Azimuth: 261, Elevation: 38}, Freqs: []SatsInfoFreq{{SysStatus: 6, SNR: 46, FreqStatus: 6, FreqNo: 1}}},
			},
		},
	},
	{
		name:        "PPSSTATUS with standard timing data",
		binPacket:   mustHexDecode("aa44b55d28233c0000a0480968e334200000000000121d000300000048090000" + "80df3420fcffffffa0b259fe2000e803150000000000000069666600000000" + "2bbcd2100100000000acecb02c000000000000000091a800a5"),
		asciiPacket: "#PPSSTATUSA,93,GPS,FINE,2376,540337000,0,0,18,29;3,2376,540336000,-4,-27676000,0x03E80020,0x00000015,0,0x00666669,0x2B000000,0x0110D2BC,0x00000000,0x2CB0ECAC,0x00000000,0x00000000*0bbaac1a\r\n",
		header: MessageHeader{
			CPUIdlePercent: 93,
			TimingHeader: TimingHeader{
				TimeRef:            TimeRefGPS,
				TimeStatus:         TimeStatusFine,
				Week:               2376,
				MillisecondsOfWeek: 540337000,
				Reserved:           0,
				Version:            0,
				LeapSec:            18,
				DelayMs:            29,
			},
		},
		value: &PPSStatus{
			Status:       3,
			Week:         2376,
			MsSecInWeek:  540336000,
			PPSPulseErr:  -4,
			OffsetTime:   -27676000,
			ConfigInfo:   0x03E80020,
			Register:     0x00000015,
			TimeEstErr:   0,
			InnerQuality: 0x00666669,
			PPSInnerSta:  0x2B000000,
			PPSCfgSta:    0x0110D2BC,
			PPSStage:     0x00000000,
			Reserved1:    0x2CB0ECAC,
		},
	},
	{
		name:        "RECTIME with timing and UTC data",
		binPacket:   mustHexDecode("aa44b56166002c0000a04b0988a70a16000000000012120000000000b39fb0a7e62a3fbfafeea64e93f94b3e00000000000032c0e9070000080e062a78e6000001000000b3998b61"),
		asciiPacket: "#RECTIMEA,97,GPS,FINE,2379,369797000,0,0,18,18;VALID,-4.755795596e-04,1.302682980e-08,-18.00000000000,2025,8,14,6,42,59000,VALID*9f81d9db\r\n",
		header: MessageHeader{
			CPUIdlePercent: 97,
			TimingHeader: TimingHeader{
				TimeRef:            TimeRefGPS,
				TimeStatus:         TimeStatusFine,
				Week:               2379,
				MillisecondsOfWeek: 369797000,
				Reserved:           0,
				Version:            0,
				LeapSec:            18,
				DelayMs:            18,
			},
		},
		value: &RecTime{
			ClockStatus: ClockStatusValid,
			Offset:      -0.00047557955957921587,
			OffsetStd:   1.3026829799647186e-08,
			UTCOffset:   -18.0,
			UTCYear:     2025,
			UTCMonth:    8,
			UTCDay:      14,
			UTCHour:     6,
			UTCMin:      42,
			UTCMs:       59000,
			UTCStatus:   UTCStatusValid,
		},
		fixupValueForAscii: fixupRecTimeValueForAscii,
	},
	{
		name:        "VERSION message",
		binPacket:   mustHexDecode("aa44b5612500340100a04b09f00e73150000000000120300120000003133353034000000000000000000000000000000000000000000000000000000004852505430302d533130432d500000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000323331303431353030303030312d4d443232413632323435313839383200000000000000000000000000000000000000000000000000000000000000000000000000666633626164393933393561396166630000000000000000000000000000000000323032342f30342f3033000000000000000000000000000000000000000000000000000000000000000000e108e8bf"),
		asciiPacket: "#VERSIONA,97,GPS,FINE,2379,359862000,0,0,18,3;\"UM980\",\"R4.10Build13504\",\"HRPT00-S10C-P\",\"2310415000001-MD22A6224518982\",\"ff3bad99395a9afc\",\"2024/04/03\"*8d58bb37\r\n",
		header: MessageHeader{
			CPUIdlePercent: 97,
			TimingHeader: TimingHeader{
				TimeRef:            TimeRefGPS,
				TimeStatus:         TimeStatusFine,
				Week:               2379,
				MillisecondsOfWeek: 359862000,
				Reserved:           0,
				Version:            0,
				LeapSec:            18,
				DelayMs:            3,
			},
		},
		value: &Version{
			Type:        ProductModelUM980,
			SwVersion:   [33]byte{'R', '4', '.', '1', '0', 'B', 'u', 'i', 'l', 'd', '1', '3', '5', '0', '4'},
			Auth:        [129]byte{'H', 'R', 'P', 'T', '0', '0', '-', 'S', '1', '0', 'C', '-', 'P'},
			Psn:         [66]byte{'2', '3', '1', '0', '4', '1', '5', '0', '0', '0', '0', '0', '1', '-', 'M', 'D', '2', '2', 'A', '6', '2', '2', '4', '5', '1', '8', '9', '8', '2'},
			EfuseID:     [33]byte{'f', 'f', '3', 'b', 'a', 'd', '9', '9', '3', '9', '5', 'a', '9', 'a', 'f', 'c'},
			CompileTime: [43]byte{'2', '0', '2', '4', '/', '0', '4', '/', '0', '3'},
		},
		fixupValueForBin: fixupVersionValueForBin,
	},
}

func TestDataBin(t *testing.T) {
	for _, tt := range dataTests {
		t.Run(tt.name, func(t *testing.T) {
			// Test parsing binary packet
			header, msg, err := ParseBinMsg(tt.binPacket)
			if err != nil {
				t.Fatalf("ParseBinMsg() error = %v", err)
			}

			if !reflect.DeepEqual(header, tt.header) {
				t.Errorf("ParseBinMsg() header mismatch:\nGot:  %+v\nWant: %+v", header, tt.header)
			}

			// Apply fixup for binary comparison if needed
			expectedValue := tt.value
			if tt.fixupValueForBin != nil {
				expectedValue = tt.fixupValueForBin(tt.value)
			}

			if !reflect.DeepEqual(msg, expectedValue) {
				t.Errorf("ParseBinMsg() mismatch:\nGot:  %+v\nWant: %+v", msg, expectedValue)
			}

			// Test round-trip: value -> serialize -> parse using expectedValue for both directions
			serialized, err := SerializeBinMsg(header, expectedValue)
			if err != nil {
				t.Fatalf("SerializeBinMsg() error = %v", err)
			}

			header2, msg2, err := ParseBinMsg(serialized)
			if err != nil {
				t.Fatalf("ParseBinMsg() on serialized packet error = %v", err)
			}

			if !reflect.DeepEqual(header2, tt.header) {
				t.Errorf("ParseBinMsg() on serialized header mismatch:\nGot:  %+v\nWant: %+v", header2, tt.header)
			}

			if !reflect.DeepEqual(msg2, expectedValue) {
				t.Errorf("ParseBinMsg() on serialized mismatch:\nGot:  %+v\nWant: %+v", msg2, expectedValue)
			}
		})
	}
}

func TestDataAscii(t *testing.T) {
	for _, tt := range dataTests {
		t.Run(tt.name, func(t *testing.T) {
			// Test parsing ASCII packet
			header, msg, err := ParseAsciiMessage([]byte(tt.asciiPacket))
			if err != nil {
				t.Fatalf("ParseAsciiMessage() error = %v", err)
			}

			if !reflect.DeepEqual(header, tt.header) {
				t.Errorf("ParseAsciiMessage() header mismatch:\nGot:  %+v\nWant: %+v", header, tt.header)
			}

			// Apply fixup for ASCII comparison if needed
			expectedValue := tt.value
			if tt.fixupValueForAscii != nil {
				expectedValue = tt.fixupValueForAscii(tt.value)
			}

			if !reflect.DeepEqual(msg, expectedValue) {
				t.Errorf("ParseAsciiMessage() mismatch:\nGot:  %+v\nWant: %+v", msg, expectedValue)
			}

			// Test round-trip: value -> serialize -> parse using expectedValue for both directions
			serialized, err := SerializeAsciiMsg(header, expectedValue)
			if err != nil {
				t.Fatalf("SerializeAsciiMsg() error = %v", err)
			}

			header2, msg2, err := ParseAsciiMessage(serialized)
			if err != nil {
				t.Fatalf("ParseAsciiMessage() on serialized packet error = %v", err)
			}

			if !reflect.DeepEqual(header2, tt.header) {
				t.Errorf("ParseAsciiMessage() on serialized header mismatch:\nGot:  %+v\nWant: %+v", header2, tt.header)
			}

			if !reflect.DeepEqual(msg2, expectedValue) {
				t.Errorf("ParseAsciiMessage() on serialized mismatch:\nGot:  %+v\nWant: %+v", msg2, expectedValue)
			}
		})
	}
}

func mustHexDecode(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// fixupRecTimeValueForAscii converts a RecTime with full binary precision
// to match the limited precision values that come from ASCII parsing.
// This simulates the receiver firmware's float-to-ASCII conversion using %.10g formatting.
func fixupRecTimeValueForAscii(msg Msg) Msg {
	r := msg.(*RecTime)
	result := *r // Copy the struct

	// Simulate receiver's ASCII formatting: binary -> printf("%.10g") -> parse back
	fixupFloat(&result.Offset)
	fixupFloat(&result.OffsetStd)
	// UTCOffset typically stays exact since -18.0 is representable exactly

	return &result
}

// fixupFloat simulates the receiver's float formatting by doing: binary -> sprintf -> parse
func fixupFloat(val *float64) {
	str := fmt.Sprintf("%.10g", *val)
	result, err := strconv.ParseFloat(str, 64)
	if err != nil {
		panic(fmt.Sprintf("failed to round-trip float64 %v: %v", *val, err))
	}
	*val = result
}
