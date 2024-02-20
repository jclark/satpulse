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

func TestInSync(t *testing.T) {
	for i, test := range inSyncTests {
		inSync := false
		w := newSampleWindow(samplesToKeep)
		for j, s := range test {
			if s.kind != sampleMissing && w.madIsOutlier(s.off, &defaultSampleConfig) != (s.kind == sampleOutlier) {
				n, min, max := w.mad(defaultSampleConfig.madMultiple)
				t.Errorf("Test %d, sample %d, expected madIsOutlier == %v (n = %d, min = %v, max = %v)", i, j, s.kind == sampleOutlier, n, min, max)
			}
			w.append(s.kind, s.off, 1)
			inSync = w.isInSync(inSync, &defaultSampleConfig)
			expectInSync := (j + 1) >= defaultSampleConfig.minGood
			if inSync != expectInSync {
				t.Errorf("Test %d, sample %d, expected isInSync == %v", i, j, expectInSync)
			}
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
		w := newSampleWindow(samplesToKeep)
		for j, off := range test {
			if w.madIsOutlier(off, &defaultSampleConfig) {
				n, min, max := w.mad(defaultSampleConfig.madMultiple)
				t.Errorf("Test %d, sample %d, expected madIsOutlier == false (n = %d, min = %v, max = %v)", i, j, n, min, max)
			}
			w.append(sampleOK, off, 1)
		}
	}
}
