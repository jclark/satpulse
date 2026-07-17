package septentrio

import (
	"math"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/sbfbin"
)

// satCombiner accumulates SVInfo while combining ChannelStatus look angles and
// used flags with MeasEpoch signals.
type satCombiner struct {
	svs    []gpsprot.SVInfo
	index  map[gpsprot.SVID]int
	used   map[gpsprot.SVID]map[string]bool
	usedSV map[gpsprot.SVID]bool
}

func (c *satCombiner) sv(id gpsprot.SVID) *gpsprot.SVInfo {
	if i, ok := c.index[id]; ok {
		return &c.svs[i]
	}
	c.index[id] = len(c.svs)
	c.svs = append(c.svs, gpsprot.SVInfo{ID: id, Signals: []gpsprot.SignalInfo{}})
	return &c.svs[len(c.svs)-1]
}

// satellitesCombine merges ChannelStatus (used + look angles) and MeasEpoch
// (signals + CN0) into one SatellitesMsg.
func satellitesCombine(chn *sbfbin.ChannelStatus, meas *sbfbin.MeasEpoch) *gpsprot.SatellitesMsg {
	c := satCombiner{
		svs:    make([]gpsprot.SVInfo, 0, len(chn.SatInfo)),
		index:  map[gpsprot.SVID]int{},
		used:   map[gpsprot.SVID]map[string]bool{},
		usedSV: map[gpsprot.SVID]bool{},
	}
	c.addChannelStatus(chn)
	validity := gpsprot.SatelliteUsedSV
	if meas != nil {
		validity = gpsprot.SatelliteUsedSignal
		c.addMeasEpoch(meas)
	}
	c.setSVUsed()
	return &gpsprot.SatellitesMsg{
		SVs:          c.svs,
		NativeMsgID:  "ChannelStatus",
		UsedValidity: validity,
	}
}

// addChannelStatus records look angles and the RINEX codes of signals used in
// the PVT solution.
func (c *satCombiner) addChannelStatus(chn *sbfbin.ChannelStatus) {
	for i := range chn.SatInfo {
		si := &chn.SatInfo[i]
		id, ok := sbfSVID(resolveSVID(uint16(si.SVID), si.SVIDFull))
		if !ok {
			continue
		}
		st := mainAntenna(chn.StateInfo[i])
		sv := c.sv(id)
		if la, ok := channelLookAngles(si); ok {
			sv.LookAngles = la
		}
		if st != nil {
			c.addUsedCodes(id, st)
		}
	}
}

// addMeasEpoch appends one signal for every Type1 master and Type2 slave
// measurement.
func (c *satCombiner) addMeasEpoch(meas *sbfbin.MeasEpoch) {
	for i := range meas.Type1 {
		t := &meas.Type1[i]
		if t.AntennaID() != 0 {
			continue
		}
		id, ok := sbfSVID(uint16(t.SVID))
		if !ok {
			continue
		}
		c.addMeasSignal(id, t.SignalNumber(), meas.CommonFlags, measCN0(t.CN0dBHz()))
		for j := range meas.Type2[i] {
			t := &meas.Type2[i][j]
			if t.AntennaID() != 0 {
				continue
			}
			c.addMeasSignal(id, t.SignalNumber(), meas.CommonFlags, measCN0(t.CN0dBHz()))
		}
	}
}

func (c *satCombiner) addUsedCodes(id gpsprot.SVID, st *sbfbin.ChannelStateInfo) {
	used := c.used[id]
	if used == nil {
		used = map[string]bool{}
		c.used[id] = used
	}
	sys := id.GNSS.SVIDPrefix()
	for slot := 0; slot < 8; slot++ {
		if st.PVTStatus.Slot(slot) != sbfbin.PVTStatusUsed {
			continue
		}
		c.usedSV[id] = true
		for _, code := range sbfbin.RINEXChannelStatusSignals(sys, slot) {
			used[code] = true
		}
	}
}

func (c *satCombiner) addMeasSignal(id gpsprot.SVID, num uint8, flags sbfbin.CommonFlags, cn0 uint8) {
	i, ok := c.index[id]
	if !ok {
		return
	}
	sys, code := sbfbin.RINEXSig(num, flags)
	if sys == "" || code == "" || sys != id.GNSS.SVIDPrefix() {
		return
	}
	sigID := gpsprot.RINEXSignalID(sys, code)
	if sigID == gpsprot.SigIDInvalid {
		return
	}
	sv := &c.svs[i]
	used := c.used[id][code]
	sv.Signals = append(sv.Signals, gpsprot.SignalInfo{ID: sigID, CN0: cn0, Used: used})
}

func (c *satCombiner) setSVUsed() {
	for i := range c.svs {
		c.svs[i].Used = c.usedSV[c.svs[i].ID]
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
// ChannelSatInfo, if both azimuth and elevation are available.
func channelLookAngles(si *sbfbin.ChannelSatInfo) (opt.Val[gpsprot.LookAngles], bool) {
	az, azOK := si.Azimuth()
	if !azOK || si.Elevation == sbfbin.ChannelElevationDNU {
		return opt.Val[gpsprot.LookAngles]{}, false
	}
	return opt.Make(gpsprot.LookAngles{Azimuth: int16(az), Elevation: si.Elevation}), true
}

func measCN0(cn0 float64, ok bool) uint8 {
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
