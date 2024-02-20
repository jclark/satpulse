package mon

import "testing"

var inSyncTests = [][]sampleData{
	{
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
			if s.kind != sampleMissing && w.madIsOutlier(s.off, &sampleCfg) != (s.kind == sampleOutlier) {
				n, min, max := w.mad(sampleCfg.madMultiple)
				t.Errorf("Test %d, sample %d, expected madIsOutlier == %v (n = %d, min = %v, max = %v)", i, j, s.kind == sampleOutlier, n, min, max)
			}
			w.append(s.kind, s.off, 1)
			inSync = w.isInSync(inSync, &sampleCfg)
			expectInSync := (j + 1) >= sampleCfg.minGood
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
}

func TestOutlierInit(t *testing.T) {
	for i, test := range initTests {
		w := newSampleWindow(samplesToKeep)
		for j, off := range test {
			if w.madIsOutlier(off, &sampleCfg) {
				t.Errorf("Test %d, sample %d, expected madIsOutlier == false", i, j)
			}
			w.append(sampleOK, off, 1)
		}
	}
}
