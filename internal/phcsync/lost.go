package phcsync

type lostSampleGenerator struct{}

func newLostSampleGenerator() *lostSampleGenerator {
	return &lostSampleGenerator{}
}

func (g *lostSampleGenerator) pulseEdgeSample(edge PulseEdge, edgeIndex uint64) *Sample {
	return nil
}

func (g *lostSampleGenerator) timeMessageSample() *Sample {
	return nil
}

type lostSampleProcessor struct{}

func newLostSampleProcessor() *lostSampleProcessor {
	return &lostSampleProcessor{}
}

func (p *lostSampleProcessor) processSample(sample *Sample) (phcAction, controllerMode) {
	return phcAction{actionType: phcNoAction}, modeLost
}