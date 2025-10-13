package phcsync

type lostSampleGenerator struct{}

func newLostSampleGenerator() *lostSampleGenerator {
	return &lostSampleGenerator{}
}

func (g *lostSampleGenerator) pulseEdgeSample(edge PulseEdge) *SampleData {
	return nil
}

func (g *lostSampleGenerator) timeMessageSample() *SampleData {
	return nil
}

type lostSampleProcessor struct{}

func newLostSampleProcessor() *lostSampleProcessor {
	return &lostSampleProcessor{}
}

func (p *lostSampleProcessor) processSample(sample *SampleData) (phcAction, controllerMode) {
	return phcAction{actionType: phcNoAction}, modeLost
}