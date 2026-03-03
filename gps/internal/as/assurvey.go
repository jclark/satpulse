package as

import (
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
)

func surveyNavSvin(m *asbin.NavSvin) *gpsprot.SurveyMsg {
	return &gpsprot.SurveyMsg{
		// Allystar NAV-SVIN does not provide position coordinates
		Accuracy:   gpsprot.Length(m.MeanStdDev) * (gpsprot.Millimeter / 10),
		Valid:      m.Valid != 0,
		InProgress: m.Status == 0,
		ObsCount:   m.PosUsed,
		ObsTime:    gpsprot.Duration(m.PosUsed) * gpsprot.Second, // approximate; one sample per second
	}
}
