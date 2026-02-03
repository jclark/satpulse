package ts

import (
	"sync/atomic"

	"github.com/jclark/satpulse/internal/phctime"
)

type atomicEra atomic.Uint64

func (c *atomicEra) inc() phctime.Era {
	return c.add(1)
}

func (c *atomicEra) add(n uint64) phctime.Era {
	return phctime.Era((*atomic.Uint64)(c).Add(n))
}

func (c *atomicEra) load() phctime.Era {
	return phctime.Era((*atomic.Uint64)(c).Load())
}
