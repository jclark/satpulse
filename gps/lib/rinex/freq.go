package rinex

// SignalFrequencyHz returns the carrier frequency in Hz for a RINEX system
// letter and signal id. frq is the GLONASS FDMA frequency channel and must be
// non-nil for GLONASS; it is ignored for other systems. It reports false when
// the system, signal band, or (for GLONASS) channel is not recognised.
func SignalFrequencyHz(sys string, sig SignalID, frq *int8) (float64, bool) {
	f, ok := signalFrequencyMHz(sys, sig, frq)
	if !ok {
		return 0, false
	}
	return f * 1e6, true
}

// FrequencyHz returns o's carrier frequency in Hz, deriving the system from the
// satellite id and the GLONASS channel from o.Frq. It reports false when the
// frequency cannot be determined.
func (o SignalObservation) FrequencyHz() (float64, bool) {
	return SignalFrequencyHz(o.System(), o.Sig, o.Frq.Ptr())
}

func signalFrequencyMHz(sys string, sig SignalID, frq *int8) (float64, bool) {
	if len(sig) != 2 {
		return 0, false
	}
	band := sig[0]
	switch sys {
	case "G":
		switch band {
		case '1':
			return 1575.420, true
		case '2':
			return 1227.600, true
		case '5':
			return 1176.450, true
		}
	case "R":
		if frq == nil {
			return 0, false
		}
		k := float64(*frq)
		switch band {
		case '1':
			return 1602.000 + k*0.5625, true
		case '2':
			return 1246.000 + k*0.4375, true
		}
	case "E":
		switch band {
		case '1':
			return 1575.420, true
		case '5':
			return 1176.450, true
		case '6':
			return 1278.750, true
		case '7':
			return 1207.140, true
		case '8':
			return 1191.795, true
		}
	case "S":
		switch band {
		case '1':
			return 1575.420, true
		case '5':
			return 1176.450, true
		}
	case "J":
		switch band {
		case '1':
			return 1575.420, true
		case '2':
			return 1227.600, true
		case '5':
			return 1176.450, true
		case '6':
			return 1278.750, true
		}
	case "C":
		switch band {
		case '1':
			return 1575.420, true
		case '2':
			return 1561.098, true
		case '5':
			return 1176.450, true
		case '6':
			return 1268.520, true
		case '7':
			return 1207.140, true
		}
	case "I":
		switch band {
		case '5':
			return 1176.450, true
		case '9':
			return 2492.028, true
		}
	}
	return 0, false
}
