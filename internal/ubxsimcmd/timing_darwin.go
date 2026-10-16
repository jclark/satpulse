package ubxsimcmd

import (
	"fmt"

	"github.com/jclark/satpulse/gps/app/ubxsim"
)

func openTiming(_ *flagVars) (*ubxsim.Timing, func(), error) {
	return nil, nil, fmt.Errorf("--timing requires Linux")
}
