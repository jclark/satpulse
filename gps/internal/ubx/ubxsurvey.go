package ubx

import (
	"math"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

func surveyNavSvin(m *ubxbin.NavSvin) *gpsprot.SurveyMsg {
	return &gpsprot.SurveyMsg{
		Position: gpsprot.Point3D{
			lengthHP(m.MeanX, m.MeanXHP),
			lengthHP(m.MeanY, m.MeanYHP),
			lengthHP(m.MeanZ, m.MeanZHP),
		},
		Accuracy:   gpsprot.Length(m.MeanAcc) * (gpsprot.Millimeter / 10),
		Valid:      m.Valid != 0,
		InProgress: m.Active != 0,
		ObsCount:   m.Obs,
		ObsTime:    gpsprot.Duration(m.Dur) * gpsprot.Second,
	}
}

func surveyTimSvin(m *ubxbin.TimSvin) *gpsprot.SurveyMsg {
	return &gpsprot.SurveyMsg{
		Position: gpsprot.Point3D{
			gpsprot.Length(m.MeanX) * gpsprot.Centimeter,
			gpsprot.Length(m.MeanY) * gpsprot.Centimeter,
			gpsprot.Length(m.MeanZ) * gpsprot.Centimeter,
		},
		Accuracy:   gpsprot.Length(math.Round(math.Sqrt(float64(m.MeanV)) * float64(gpsprot.Millimeter))),
		Valid:      m.Valid != 0,
		InProgress: m.Active != 0,
		ObsCount:   m.Obs,
		ObsTime:    gpsprot.Duration(m.Dur) * gpsprot.Second,
	}
}
