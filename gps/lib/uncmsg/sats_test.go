package uncmsg

import (
	"testing"
)

var satsTests = []dataTestCase{
	{
		name:        "SatsInfo with 47 satellites",
		binPacket:   mustHexDecode("aa44b55e4c08f60200a04909a058d9000000000000121c002f020000003f18af00130020000400251104002a0e04002409041d3c0133002c000300281103002c090319ea0013002800040025110400240e04002809040cc900140028000300211103002609030fe0004f003100030030110300310903170d0112002000050020110500240e0500210305001d09050d41003c001c0001c2950013052600040525110405260e0405280304c774003805250004052c1104052d0e0405270304c4360035051a1103051b0e0305190303c3490031051a0e013b75001701220002011d07022d150017011a00020116050227e2002a012e0002012b05023c460033011a00013d5c011c011b0003011a05030120070326440146012b00013cf2003e04320003042e150304310d0302e90042042b000304321103042c1503038c004704300003042e1103042b150305ff0027042a0003042b1103042a15030d4e012c04240003042411030422150325c0000804240005042415050421080504260c0504280d0514e6000d04290005042515050426080504230c0504250d052791001c042a0005042615050428080504250c0504260d0506a0001804240003042c11030428150309ad001304220003042711030422150328a00037042b0005042e1505042a0805042a0c05042d0d0507b0003c042d0003042d1103042c15031c6d0036042b0005042715050429080504260c0504290d050aca00450430000304301103042e1503109e0017042800030428110304221503085b012a041900030420110304191503200f013e04300005042f1505042f080504300c0504320d051b03004104270005042615050427080504240c0504260d05242b0029031902011e19010f031d02040325110403220c0403201604092b001d03181101056c004a033002040334110403310c04033016042252011e031a11020317160219b0000d031c020303180c030317160303c80025032c0204032f1104032b0c040329160402e4001b032c0204032d1104032b0c04032a160428750034062c060180f40022063006013e04010c06260601ba050126062e0601b62e3613"),
		asciiPacket: "#SATSINFOA,94,GPS,FINE,2377,14244000,0,0,18,28;47,2,0,0,0,63,24,175,19,0,32,0,4,0,37,17,4,0,42,14,4,0,36,9,4,29,316,51,0,44,0,3,0,40,17,3,0,44,9,3,25,234,19,0,40,0,4,0,37,17,4,0,36,14,4,0,40,9,4,12,201,20,0,40,0,3,0,33,17,3,0,38,9,3,15,224,79,0,49,0,3,0,48,17,3,0,49,9,3,23,269,18,0,32,0,5,0,32,17,5,0,36,14,5,0,33,3,5,0,29,9,5,13,65,60,0,28,0,1,194,149,19,5,38,0,4,5,37,17,4,5,38,14,4,5,40,3,4,199,116,56,5,37,0,4,5,44,17,4,5,45,14,4,5,39,3,4,196,54,53,5,26,17,3,5,27,14,3,5,25,3,3,195,73,49,5,26,14,1,59,117,23,1,34,0,2,1,29,7,2,45,21,23,1,26,0,2,1,22,5,2,39,226,42,1,46,0,2,1,43,5,2,60,70,51,1,26,0,1,61,348,28,1,27,0,3,1,26,5,3,1,32,7,3,38,324,70,1,43,0,1,60,242,62,4,50,0,3,4,46,21,3,4,49,13,3,2,233,66,4,43,0,3,4,50,17,3,4,44,21,3,3,140,71,4,48,0,3,4,46,17,3,4,43,21,3,5,255,39,4,42,0,3,4,43,17,3,4,42,21,3,13,334,44,4,36,0,3,4,36,17,3,4,34,21,3,37,192,8,4,36,0,5,4,36,21,5,4,33,8,5,4,38,12,5,4,40,13,5,20,230,13,4,41,0,5,4,37,21,5,4,38,8,5,4,35,12,5,4,37,13,5,39,145,28,4,42,0,5,4,38,21,5,4,40,8,5,4,37,12,5,4,38,13,5,6,160,24,4,36,0,3,4,44,17,3,4,40,21,3,9,173,19,4,34,0,3,4,39,17,3,4,34,21,3,40,160,55,4,43,0,5,4,46,21,5,4,42,8,5,4,42,12,5,4,45,13,5,7,176,60,4,45,0,3,4,45,17,3,4,44,21,3,28,109,54,4,43,0,5,4,39,21,5,4,41,8,5,4,38,12,5,4,41,13,5,10,202,69,4,48,0,3,4,48,17,3,4,46,21,3,16,158,23,4,40,0,3,4,40,17,3,4,34,21,3,8,347,42,4,25,0,3,4,32,17,3,4,25,21,3,32,271,62,4,48,0,5,4,47,21,5,4,47,8,5,4,48,12,5,4,50,13,5,27,3,65,4,39,0,5,4,38,21,5,4,39,8,5,4,36,12,5,4,38,13,5,36,43,41,3,25,2,1,30,281,15,3,29,2,4,3,37,17,4,3,34,12,4,3,32,22,4,9,43,29,3,24,17,1,5,108,74,3,48,2,4,3,52,17,4,3,49,12,4,3,48,22,4,34,338,30,3,26,17,2,3,23,22,2,25,176,13,3,28,2,3,3,24,12,3,3,23,22,3,3,200,37,3,44,2,4,3,47,17,4,3,43,12,4,3,41,22,4,2,228,27,3,44,2,4,3,45,17,4,3,43,12,4,3,42,22,4,40,117,52,6,44,6,1,128,244,34,6,48,6,1,62,260,12,6,38,6,1,186,261,38,6,46,6,1*5f93d72e\r\n",
		msg: &Msg{
			Hdr: MsgHdr{
				CPUIdlePercent: 94,
				TimingHdr: TimingHdr{
					TimeRef:            TimeRefGPS,
					TimeStatus:         TimeStatusFine,
					Week:               2377,
					MillisecondsOfWeek: 14244000,
					Version:            0,
					Reserved:           0,
					LeapSec:            18,
					DelayMs:            28,
				},
			},
			Body: &SatsInfo{
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
	},
	{
		name:        "BestSat with 34 satellites",
		binPacket:   mustHexDecode("aa44b5601104240200a04d09882573168c4400000012160022000000000000000000020000000000030000000000000000000a000000000007000000000000000000170000000000070000000000000000001a0000000000070000000000000000001c0000000000070000000000000000001f000000000003000000070000000000c3000000000007000000070000000000c7000000000007000000010000000c002800000000000100000001000000000033000000000003000000010000000700340000000000030000000100000006003500000000000300000001000000040037000000000003000000010000000a0038000000000003000000050000000000040000000000070000000500000000000a0000000000070000000500000000000c000000000007000000050000000000170000000000070000000500000000001f000000000007000000050000000000210000000000070000000600000000000200000000000700000006000000000003000000000007000000060000000000060000000000070000000600000000000700000000000700000006000000000008000000000007000000060000000000090000000000070000000600000000000a0000000000070000000600000000000d000000000007000000060000000000100000000000070000000600000000001800000000006d0000000600000000001900000000006d0000000600000000002600000000006d0000000600000000002800000000006d0000000600000000002b00000000006d00000081303601"),
		asciiPacket: "#BESTSATA,96,GPS,FINE,2381,376645000,17548,0,18,22;34,GPS,2,GOOD,00000003,GPS,10,GOOD,00000007,GPS,23,GOOD,00000007,GPS,26,GOOD,00000007,GPS,28,GOOD,00000007,GPS,31,GOOD,00000003,QZSS,195,GOOD,00000007,QZSS,199,GOOD,00000007,GLONASS,40+12,GOOD,00000001,GLONASS,51,GOOD,00000003,GLONASS,52+7,GOOD,00000003,GLONASS,53+6,GOOD,00000003,GLONASS,55+4,GOOD,00000003,GLONASS,56+10,GOOD,00000003,GALILEO,4,GOOD,00000007,GALILEO,10,GOOD,00000007,GALILEO,12,GOOD,00000007,GALILEO,23,GOOD,00000007,GALILEO,31,GOOD,00000007,GALILEO,33,GOOD,00000007,BEIDOU,2,GOOD,00000007,BEIDOU,3,GOOD,00000007,BEIDOU,6,GOOD,00000007,BEIDOU,7,GOOD,00000007,BEIDOU,8,GOOD,00000007,BEIDOU,9,GOOD,00000007,BEIDOU,10,GOOD,00000007,BEIDOU,13,GOOD,00000007,BEIDOU,16,GOOD,00000007,BEIDOU,24,GOOD,0000006d,BEIDOU,25,GOOD,0000006d,BEIDOU,38,GOOD,0000006d,BEIDOU,40,GOOD,0000006d,BEIDOU,43,GOOD,0000006d*c48b5090\r\n",
		msg: &Msg{
			Hdr: MsgHdr{
				CPUIdlePercent: 96,
				TimingHdr: TimingHdr{
					TimeRef:            TimeRefGPS,
					TimeStatus:         TimeStatusFine,
					Week:               2381,
					MillisecondsOfWeek: 376645000,
					Version:            17548,
					Reserved:           0,
					LeapSec:            18,
					DelayMs:            22,
				},
			},
			Body: &BestSat{
				BestSatInitChunk: BestSatInitChunk{NumEntries: 34},
				Sats: []BestSatEntry{
					{System: SatSysGPS, SatID: MakeSatID(2), Status: SatStatusGood, SigMask: SigUsed(0x00000003)},
					{System: SatSysGPS, SatID: MakeSatID(10), Status: SatStatusGood, SigMask: SigUsed(0x00000007)},
					{System: SatSysGPS, SatID: MakeSatID(23), Status: SatStatusGood, SigMask: SigUsed(0x00000007)},
					{System: SatSysGPS, SatID: MakeSatID(26), Status: SatStatusGood, SigMask: SigUsed(0x00000007)},
					{System: SatSysGPS, SatID: MakeSatID(28), Status: SatStatusGood, SigMask: SigUsed(0x00000007)},
					{System: SatSysGPS, SatID: MakeSatID(31), Status: SatStatusGood, SigMask: SigUsed(0x00000003)},
					{System: SatSysQZSS, SatID: MakeSatID(195), Status: SatStatusGood, SigMask: SigUsed(0x00000007)},
					{System: SatSysQZSS, SatID: MakeSatID(199), Status: SatStatusGood, SigMask: SigUsed(0x00000007)},
					{System: SatSysGLONASS, SatID: MakeSatID(40, 12), Status: SatStatusGood, SigMask: SigUsed(0x00000001)},
					{System: SatSysGLONASS, SatID: MakeSatID(51), Status: SatStatusGood, SigMask: SigUsed(0x00000003)},
					{System: SatSysGLONASS, SatID: MakeSatID(52, 7), Status: SatStatusGood, SigMask: SigUsed(0x00000003)},
					{System: SatSysGLONASS, SatID: MakeSatID(53, 6), Status: SatStatusGood, SigMask: SigUsed(0x00000003)},
					{System: SatSysGLONASS, SatID: MakeSatID(55, 4), Status: SatStatusGood, SigMask: SigUsed(0x00000003)},
					{System: SatSysGLONASS, SatID: MakeSatID(56, 10), Status: SatStatusGood, SigMask: SigUsed(0x00000003)},
					{System: SatSysGALILEO, SatID: MakeSatID(4), Status: SatStatusGood, SigMask: SigUsed(0x00000007)},
					{System: SatSysGALILEO, SatID: MakeSatID(10), Status: SatStatusGood, SigMask: SigUsed(0x00000007)},
					{System: SatSysGALILEO, SatID: MakeSatID(12), Status: SatStatusGood, SigMask: SigUsed(0x00000007)},
					{System: SatSysGALILEO, SatID: MakeSatID(23), Status: SatStatusGood, SigMask: SigUsed(0x00000007)},
					{System: SatSysGALILEO, SatID: MakeSatID(31), Status: SatStatusGood, SigMask: SigUsed(0x00000007)},
					{System: SatSysGALILEO, SatID: MakeSatID(33), Status: SatStatusGood, SigMask: SigUsed(0x00000007)},
					{System: SatSysBEIDOU, SatID: MakeSatID(2), Status: SatStatusGood, SigMask: SigUsed(0x00000007)},
					{System: SatSysBEIDOU, SatID: MakeSatID(3), Status: SatStatusGood, SigMask: SigUsed(0x00000007)},
					{System: SatSysBEIDOU, SatID: MakeSatID(6), Status: SatStatusGood, SigMask: SigUsed(0x00000007)},
					{System: SatSysBEIDOU, SatID: MakeSatID(7), Status: SatStatusGood, SigMask: SigUsed(0x00000007)},
					{System: SatSysBEIDOU, SatID: MakeSatID(8), Status: SatStatusGood, SigMask: SigUsed(0x00000007)},
					{System: SatSysBEIDOU, SatID: MakeSatID(9), Status: SatStatusGood, SigMask: SigUsed(0x00000007)},
					{System: SatSysBEIDOU, SatID: MakeSatID(10), Status: SatStatusGood, SigMask: SigUsed(0x00000007)},
					{System: SatSysBEIDOU, SatID: MakeSatID(13), Status: SatStatusGood, SigMask: SigUsed(0x00000007)},
					{System: SatSysBEIDOU, SatID: MakeSatID(16), Status: SatStatusGood, SigMask: SigUsed(0x00000007)},
					{System: SatSysBEIDOU, SatID: MakeSatID(24), Status: SatStatusGood, SigMask: SigUsed(0x0000006d)},
					{System: SatSysBEIDOU, SatID: MakeSatID(25), Status: SatStatusGood, SigMask: SigUsed(0x0000006d)},
					{System: SatSysBEIDOU, SatID: MakeSatID(38), Status: SatStatusGood, SigMask: SigUsed(0x0000006d)},
					{System: SatSysBEIDOU, SatID: MakeSatID(40), Status: SatStatusGood, SigMask: SigUsed(0x0000006d)},
					{System: SatSysBEIDOU, SatID: MakeSatID(43), Status: SatStatusGood, SigMask: SigUsed(0x0000006d)},
				},
			},
		},
	},
}

func TestSats(t *testing.T) {
	t.Run("bin", func(t *testing.T) {
		testDataBin(t, satsTests)
	})
	t.Run("ascii", func(t *testing.T) {
		testDataAscii(t, satsTests)
	})
}

func TestSatID(t *testing.T) {
	tests := []struct {
		s    string
		want SatID
	}{
		{"10", MakeSatID(10)},
		{"13+4", MakeSatID(13, 4)},
	}
	for _, tt := range tests {
		got, err := ParseSatID(tt.s)
		if err != nil {
			t.Errorf("ParseSatID(%q) error = %v", tt.s, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseSatID(%q) = %v, want %v", tt.s, got, tt.want)
		}
		if got.String() != tt.s {
			t.Errorf("SatID(%d).String() = %q, want %q", got, got.String(), tt.s)
		}
	}
}
