package ubx

import (
	"time"

	"github.com/jclark/gps4ptp/internal/gpsmsg"
	"github.com/jclark/gps4ptp/internal/ubx/bin"
)

type Config struct {
	protVer ProtVer
	tmode2  *bin.CfgTmode2
	tmode3  *bin.CfgTmode3
	tp5     *bin.CfgTp5
	gnss    *bin.CfgGNSS
	rate    *bin.CfgRate
	msgRate map[bin.MsgID][6]byte
}

// Verify that Config implements gpsmsg.Config
var _ gpsmsg.Config = (*Config)(nil)

func (c *Config) EnabledGNSS() []gpsmsg.MajorGNSS {
	if c == nil || c.gnss == nil {
		return nil
	}
	enabled := make([]gpsmsg.MajorGNSS, 0)
	for _, blk := range c.gnss.Blocks {
		if blk.Enable != 0 {
			if g, ok := majorGNSS(blk.GNSSID); ok {
				enabled = append(enabled, g)
			}
		}
	}
	return enabled
}

func (c *Config) SetProtVer(protVer ProtVer) {
	c.protVer = protVer
}

func (c *Config) TimeMode() *gpsmsg.TimeMode {
	if c == nil {
		return nil
	}
	if c.tmode2 != nil {
		return timeMode2(c.tmode2)
	}
	if c.tmode3 != nil {
		return timeMode3(c.tmode3)
	}
	return nil
}

func (c *Config) SolutionPeriod() time.Duration {
	if c == nil || c.rate == nil {
		return 0
	}
	period := time.Duration(c.rate.MeasRate) * time.Millisecond
	if c.protVerAtLeast(18, 0) && period != 0 {
		period /= time.Duration(c.rate.NavRate)
	}
	return period
}

func (cfg *Config) AddMsg(m bin.Msg) bool {
	if cfg == nil {
		return false
	}
	switch mt := m.(type) {
	case *bin.CfgTmode2:
		cfg.tmode2 = mt
	case *bin.CfgTmode3:
		cfg.tmode3 = mt
	case *bin.CfgTp5:
		cfg.tp5 = mt
	case *bin.CfgGNSS:
		cfg.gnss = mt
	case *bin.CfgRate:
		cfg.rate = mt
	case *bin.CfgMsg:
		cfg.addMsgRate(mt.MsgID, mt.Rate)
	default:
		return false
	}
	return true
}

func (cfg *Config) addMsgRate(msgID bin.MsgID, rate [6]byte) {
	if cfg.msgRate == nil {
		cfg.msgRate = make(map[bin.MsgID][6]byte)
	}
	cfg.msgRate[msgID] = rate
}

func (c *Config) protVerAtLeast(major, minor byte) bool {
	return c.protVer.Major > major || (c.protVer.Major == major && c.protVer.Minor >= minor)
}

func majorGNSS(g bin.GNSSID) (gpsmsg.MajorGNSS, bool) {
	switch g {
	case bin.GPS:
		return gpsmsg.GPS, true
	case bin.GLONASS:
		return gpsmsg.GLONASS, true
	case bin.BeiDou:
		return gpsmsg.BeiDou, true
	case bin.Galileo:
		return gpsmsg.Galileo, true
	}
	return 0, false
}
