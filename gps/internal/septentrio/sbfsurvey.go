package septentrio

import (
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/sbfbin"
)

// surveyPVTCartesian builds a SurveyMsg from PVTCartesian's Mode bits. SBF has
// no dedicated self-survey block, so this is only emitted when the wire shows
// survey activity: a fixed-location solution (Valid) or the
// still-determining-fixed-position bit (InProgress). Ordinary rover epochs
// produce nil, so no meaningless "no survey" message is emitted each epoch.
//
// Without local config state this cannot tell an auto-survey fixed position
// from a manually-entered one; both report Mode fixed-location. ObsCount and
// ObsTime have no SBF source and are left unset. Accuracy uses HAccuracy
// (95%-class), the fallback source, since the 1-sigma PosCovCartesian primary
// is a separate block not threaded here.
func surveyPVTCartesian(m *sbfbin.PVTCartesian) *gpsprot.SurveyMsg {
	valid := m.Mode.Fix() == sbfbin.ModeFixedLocation
	inProgress := m.Mode.DeterminingFixed()
	if !valid && !inProgress {
		return nil
	}
	sv := &gpsprot.SurveyMsg{Valid: valid, InProgress: inProgress}
	if m.X != sbfbin.PVTCoordinateDNU && m.Y != sbfbin.PVTCoordinateDNU && m.Z != sbfbin.PVTCoordinateDNU {
		sv.Position = gpsprot.Point3D{gpsprot.Meters(m.X), gpsprot.Meters(m.Y), gpsprot.Meters(m.Z)}
	}
	if m.HAccuracy != sbfbin.PVTAccuracyDNU {
		sv.Accuracy = gpsprot.Length(m.HAccuracy) * gpsprot.Centimeter
	}
	return sv
}
