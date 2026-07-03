package septentrio

import (
	"math"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/sbfbin"
)

// satCombiner accumulates SVInfo while combining the three per-satellite
// blocks, keeping the SVID index and the ChannelStatus (SVID, MeasEpoch signal
// number) -> signal-slot join for the MeasEpoch overlay.
type satCombiner struct {
	svs   []gpsprot.SVInfo
	index map[gpsprot.SVID]int
	join  map[gpsprot.SVID]map[uint8]int
}

func (c *satCombiner) sv(id gpsprot.SVID) *gpsprot.SVInfo {
	if i, ok := c.index[id]; ok {
		return &c.svs[i]
	}
	c.index[id] = len(c.svs)
	c.svs = append(c.svs, gpsprot.SVInfo{ID: id})
	return &c.svs[len(c.svs)-1]
}

// satellitesCombine merges ChannelStatus (structural base + Used), MeasEpoch
// (CN0, precise SignalID) and SatVisibility (finer look angles + orbit-visible
// SVs) into one SatellitesMsg. It returns nil when no SV is present.
func satellitesCombine(chn *sbfbin.ChannelStatus, meas *sbfbin.MeasEpoch, vis *sbfbin.SatVisibility) *gpsprot.SatellitesMsg {
	c := satCombiner{index: map[gpsprot.SVID]int{}, join: map[gpsprot.SVID]map[uint8]int{}}
	nativeID := ""
	validity := gpsprot.SatelliteUsedInvalid
	if chn != nil {
		nativeID = "ChannelStatus"
		validity = gpsprot.SatelliteUsedSignal
		c.addChannelStatus(chn)
	}
	if meas != nil {
		if nativeID == "" {
			nativeID = "MeasEpoch"
		}
		c.overlayMeasEpoch(meas, chn != nil)
	}
	if vis != nil {
		if nativeID == "" {
			nativeID = "SatVisibility"
		}
		c.overlaySatVisibility(vis)
	}
	if len(c.svs) == 0 {
		return nil
	}
	return &gpsprot.SatellitesMsg{
		SVs:          c.svs,
		NativeMsgID:  nativeID,
		UsedValidity: validity,
	}
}

// addChannelStatus builds the structural base: one SVInfo per satellite, one
// SignalInfo per family slot being tracked, with Used from PVTStatus.
func (c *satCombiner) addChannelStatus(chn *sbfbin.ChannelStatus) {
	for i := range chn.SatInfo {
		si := &chn.SatInfo[i]
		id, ok := sbfSVID(resolveSVID(uint16(si.SVID), si.SVIDFull))
		if !ok {
			continue
		}
		fams := chanFamilies[id.GNSS]
		st := mainAntenna(chn.StateInfo[i])
		sv := c.sv(id)
		if st != nil && fams != nil {
			jm := c.join[id]
			if jm == nil {
				jm = map[uint8]int{}
				c.join[id] = jm
			}
			for slot, fam := range fams {
				if fam.id == gpsprot.SigIDInvalid || st.TrackingStatus.Slot(slot) != sbfbin.TrackStatusTracking {
					continue
				}
				used := st.PVTStatus.Slot(slot) == sbfbin.PVTStatusUsed
				sv.Signals = append(sv.Signals, gpsprot.SignalInfo{ID: fam.id, Used: used})
				if used {
					sv.Used = true
				}
				if fam.sigNum != noJoin {
					jm[fam.sigNum] = len(sv.Signals) - 1
				}
			}
		}
		if la, ok := channelLookAngles(si); ok {
			sv.LookAngles = la
		}
	}
}

// overlayMeasEpoch overlays CN0 and precise SignalID from MeasEpoch. When
// ChannelStatus contributed (haveBase), it only updates matching slots; when
// it is the base, it adds one SignalInfo per measurement.
func (c *satCombiner) overlayMeasEpoch(meas *sbfbin.MeasEpoch, haveBase bool) {
	for i := range meas.Type1 {
		t := &meas.Type1[i]
		num := t.SignalNumber()
		sigID, ok := measEpochSignalID(num, meas.CommonFlags)
		if !ok {
			continue
		}
		id, ok := sbfSVID(uint16(t.SVID))
		if !ok {
			continue
		}
		cn0 := measCN0(t)
		if haveBase {
			if jm := c.join[id]; jm != nil {
				if k, ok := jm[num]; ok {
					c.svs[c.index[id]].Signals[k].ID = sigID
					c.svs[c.index[id]].Signals[k].CN0 = cn0
				}
			}
			continue
		}
		sv := c.sv(id)
		sv.Signals = append(sv.Signals, gpsprot.SignalInfo{ID: sigID, CN0: cn0})
	}
}

// overlaySatVisibility overlays finer look angles onto known SVs and appends
// orbit-visible-but-unallocated satellites with empty signals.
func (c *satCombiner) overlaySatVisibility(vis *sbfbin.SatVisibility) {
	for i := range vis.SatInfo {
		si := &vis.SatInfo[i]
		id, ok := sbfSVID(resolveSVID(uint16(si.SVID), si.SVIDFull))
		if !ok {
			continue
		}
		sv := c.sv(id)
		if la, ok := visibilityLookAngles(si); ok {
			sv.LookAngles = la
		}
	}
}

// resolveSVID returns svid, or svidFull when svid is the zero escape.
func resolveSVID(svid, svidFull uint16) uint16 {
	if svid == 0 {
		return svidFull
	}
	return svid
}

// mainAntenna returns the ChannelStateInfo for the main antenna (0), or the
// first entry if none is explicitly the main antenna.
func mainAntenna(states []sbfbin.ChannelStateInfo) *sbfbin.ChannelStateInfo {
	for i := range states {
		if states[i].Antenna == 0 {
			return &states[i]
		}
	}
	if len(states) > 0 {
		return &states[0]
	}
	return nil
}

// channelLookAngles returns the whole-degree look angles from a
// ChannelSatInfo, if either azimuth or elevation is available.
func channelLookAngles(si *sbfbin.ChannelSatInfo) (opt.Val[gpsprot.LookAngles], bool) {
	az, azOK := si.Azimuth()
	elOK := si.Elevation != sbfbin.ChannelElevationDNU
	if !azOK && !elOK {
		return opt.Val[gpsprot.LookAngles]{}, false
	}
	var la gpsprot.LookAngles
	if azOK {
		la.Azimuth = int16(az)
	}
	if elOK {
		la.Elevation = si.Elevation
	}
	return opt.Make(la), true
}

// visibilityLookAngles returns the 0.01-degree look angles from a
// SatVisibility SatInfo.
func visibilityLookAngles(si *sbfbin.SatInfo) (opt.Val[gpsprot.LookAngles], bool) {
	azOK := si.Azimuth != sbfbin.SatVisibilityAzimuthDNU
	elOK := si.Elevation != sbfbin.SatVisibilityElevationDNU
	if !azOK && !elOK {
		return opt.Val[gpsprot.LookAngles]{}, false
	}
	var la gpsprot.LookAngles
	if azOK {
		la.Azimuth = int16(math.Round(float64(si.Azimuth) * 0.01))
	}
	if elOK {
		la.Elevation = int8(math.Round(float64(si.Elevation) * 0.01))
	}
	return opt.Make(la), true
}

// measCN0 converts a MeasEpoch master channel C/N0 to a rounded uint8.
func measCN0(t *sbfbin.MeasEpochChannelType1) uint8 {
	cn0, ok := t.CN0dBHz()
	if !ok {
		return 0
	}
	if cn0 < 0 {
		return 0
	}
	if cn0 > 255 {
		return 255
	}
	return uint8(math.Round(cn0))
}
