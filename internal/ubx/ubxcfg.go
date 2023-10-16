package ubx

import (
	"time"

	"github.com/jclark/gps4ptp/internal/gpsmsg"
	"github.com/jclark/gps4ptp/internal/ubx/bin"
)

type Config struct {
	ver    *Version
	tmode2 *bin.CfgTmode2
	tmode3 *bin.CfgTmode3
	tp5    *bin.CfgTp5
	gnss   *bin.CfgGNSS
	rate   *bin.CfgRate
}

// Verify that Config implements gpsmsg.Config
var _ gpsmsg.Config = (*Config)(nil)

func NewConfig(ver *Version) *Config {
	if ver == nil {
		panic("nil version")
	}
	return &Config{ver: ver}
}

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

func (c *Config) Version() *Version {
	if c == nil {
		return nil
	}
	return c.ver
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

func (c *Config) GetterMsgs() [][]byte {
	packets := [][]byte{
		bin.Poll(bin.CfgGNSSID),
		bin.Poll(bin.CfgRateID),
		bin.Poll(bin.CfgTp5ID),
	}
	tmodeID := bin.CfgTmode2ID
	if c.productCategory() == "HPG" {
		tmodeID = bin.CfgTmode3ID
	}
	packets = append(packets, bin.Poll(tmodeID))
	return packets
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
	default:
		return false
	}
	return true
}

func (c *Config) productCategory() string {
	if fw := c.ver.FW; fw != nil {
		return fw.ProductCategory
	}
	return ""
}

func (c *Config) protVerAtLeast(major, minor byte) bool {
	if c.ver.Prot == nil {
		return false
	}
	return c.ver.Prot.Major > major || (c.ver.Prot.Major == major && c.ver.Prot.Minor >= minor)
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
