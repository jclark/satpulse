package mon

import "testing"

var inSyncTests = [][]sampleData{
	{
		sampleData{-23e-9, sampleOK},
		sampleData{14e-9, sampleOK},
		sampleData{-5e-9, sampleOK},
		sampleData{7e-9, sampleOK},
		sampleData{-14e-9, sampleOK},
		sampleData{20e-9, sampleOK},
		sampleData{3e-9, sampleOK},
		sampleData{-4e-9, sampleOK},
		sampleData{-19e-9, sampleOK},
		sampleData{12e-9, sampleOK},
		sampleData{-4e-9, sampleOK},
		sampleData{-1e6, sampleOutlier},
		sampleData{0, sampleMissing},
		sampleData{1e3, sampleOutlier},
		sampleData{10e-9, sampleOK},
		sampleData{-4e-9, sampleOK},
	},
	{
		sampleData{-23e-9, sampleOK},
		sampleData{14e-9, sampleOK},
		sampleData{-5e-9, sampleOK},
		sampleData{7e-9, sampleOK},
		sampleData{-14e-9, sampleOK},
		sampleData{20e-9, sampleOK},
		sampleData{3e-9, sampleOK},
		sampleData{-4e-9, sampleOK},
		sampleData{-19e-9, sampleOK},
		sampleData{12e-9, sampleOK},
		sampleData{-4e-9, sampleOK},
		sampleData{49e-9, sampleOK},
		sampleData{3e-9, sampleOK},
		sampleData{-4e-9, sampleOK},
		sampleData{-1e6, sampleOutlier},
		sampleData{0, sampleMissing},
		sampleData{1e3, sampleOutlier},
		sampleData{10e-9, sampleOK},
		sampleData{-4e-9, sampleOK},
		sampleData{0, sampleMissing},
		sampleData{0, sampleMissing},
		sampleData{15e-9, sampleOK},
		sampleData{-4e-9, sampleOK},
		sampleData{20e-9, sampleOK},
		sampleData{3e-9, sampleOK},
		sampleData{-4e-9, sampleOK},
		sampleData{-2e6, sampleOutlier},
		sampleData{-2e6, sampleOutlier},
		sampleData{19e-9, sampleOK},
		sampleData{-2e-9, sampleOK},
	},
}

func TestSyncState(t *testing.T) {
	for i, test := range inSyncTests {
		ss := newSampleState(sampleWindowSize)
		for j, s := range test {
			if s.kind != sampleMissing && ss.madIsOutlier(s.off, &defaultSyncConfig) != (s.kind == sampleOutlier) {
				n, min, max := ss.mad(defaultSyncConfig.madMultiple)
				t.Errorf("Test %d, sample %d, expected madIsOutlier == %v (n = %d, min = %v, max = %v)", i, j, s.kind == sampleOutlier, n, min, max)
			}
			ss.sample(s.kind, s.off, 1, inSync, &defaultSyncConfig)
			state := ss.nextSyncState(inSync, &defaultSyncConfig)
			if state == noSync {
				t.Errorf("Test %d, sample %d, unexpected exit from sync", i, j)
			}
		}
	}
}

var exitSyncTests = [][]int{
	{20, -19, 5, 100, 101, 99, 98, 55, 120, 125, 130, 100, 88},
	{20, -19, 5, 51, 60, 65, 49, 48, 52, 43, 45, 66, 70, 55, -66, 51, 49, 65, 70, 60, 63, 62, 55},
}

func TestExitSyncEMA(t *testing.T) {
	cfg := defaultSyncConfig
	cfg.maxOffset = 50e-9
	cfg.emaAlpha = 0.1
	for i, test := range exitSyncTests {
		ss := newSampleState(sampleWindowSize)
		state := inSync
		for _, off := range test {
			ss.sample(sampleOK, float64(off)*1e-9, 1, state, &cfg)
			state = ss.nextSyncState(state, &cfg)
			if state == noSync {
				break
			}
		}
		if state == inSync {
			t.Errorf("Test %d, failed to exit sync", i)
		}
	}
}

func TestExitSyncMissing(t *testing.T) {
	cfg := defaultSyncConfig
	cfg.maxConsecInvalid = 7
	ss := newSampleState(sampleWindowSize)
	state := inSync
	for i := range(cfg.maxConsecInvalid + 1) {
		ss.sample(sampleMissing, 0, 1, state, &cfg)
		state = ss.nextSyncState(state, &cfg)
		if (state == noSync) != (i == cfg.maxConsecInvalid) {
			t.Errorf("Exit sync at wrong time: sample %d, state %v, invalidWeight = %v", i, state, ss.invalidWeight)
		}
	}
}

var initTests = [][]float64{
	{
		258e-9,
		255e-9,
		261e-9,
		250e-9,
		255e-9,
		229e-9,
		207e-9,
		195e-9,
		174e-9,
		162e-9,
		13e-9,
		-71e-9,
		-161e-9,
		-86e-9,
		-40e-9,
		-6e-9,
		7e-9,
	},
	{
		//-4595e-9,
		//-4612e-9,
		//-4637e-9,
		-4646e-9,
		-4671e-9,
		-4711e-9,
		-4710e-9,
		-1003e-9,
		3706e-9,
		8407e-9,
		13107e-9,
		17800e-9,
		22493e-9,
		27193e-9,
		31902e-9,
		2356e-9,
		-8947e-9,
		-9371e-9,
		-6344e-9,
		-3365e-9,
		-1399e-9,
		-391e-9,
		5e-9,
		106e-9,
		80e-9,
		31e-9,
		10e-9,
		3e-9,
	},
}

func TestOutlierInit(t *testing.T) {
	for i, test := range initTests {
		ss := newSampleState(sampleWindowSize)
		for j, off := range test {
			if ss.madIsOutlier(off, &defaultSyncConfig) {
				n, min, max := ss.mad(defaultSyncConfig.madMultiple)
				t.Errorf("Test %d, sample %d, expected madIsOutlier == false (n = %d, min = %v, max = %v)", i, j, n, min, max)
			}
			ss.win.append(sampleData{off: off, kind: sampleOK})
		}
	}
}
