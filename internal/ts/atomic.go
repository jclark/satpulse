package ts

import (
	"sync/atomic"

	"github.com/jclark/satpulse/internal/ptime"
)

type atomicEra atomic.Uint64

func (c *atomicEra) inc() ptime.Era {
	return c.add(1)
}

func (c *atomicEra) add(n uint64) ptime.Era {
	return ptime.Era((*atomic.Uint64)(c).Add(n))
}

func (c *atomicEra) load() ptime.Era {
	return ptime.Era((*atomic.Uint64)(c).Load())
}
