package gpscmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jclark/satpulse/gps/app/gpscfg"
	"github.com/jclark/satpulse/gps/gpsprot"
)

// jsonOutput is the single object written to stdout by --json.
type jsonOutput struct {
	Receiver      *gpsprot.ReceiverInfo       `json:"receiver,omitempty"`
	Supports      *gpsprot.ConfigSupportFlags `json:"supports,omitempty"`
	PacketFormats []gpsprot.Tag               `json:"packetFormats,omitempty"`
	Config        *gpsprot.ConfigProps        `json:"config,omitempty"`
	Error         string                      `json:"error,omitempty"`
}

// printJSON writes the configuration result as a single JSON object.
// On error it writes whatever partial results exist with the error field set.
func printJSON(f *os.File, rslt *gpscfg.Result, err error) {
	var out jsonOutput
	if rslt != nil {
		out.Receiver = rslt.ReceiverInfo
		out.Supports = &rslt.ConfigSupport
		out.PacketFormats = rslt.PacketFormatsDetected
		if rslt.ConfigProps != nil && !rslt.ConfigProps.IsEmpty() {
			out.Config = rslt.ConfigProps
		}
	}
	if err != nil {
		out.Error = err.Error()
	}
	b, e := json.Marshal(&out)
	if e != nil {
		panic(e) // all field types marshal without error
	}
	fmt.Fprintf(f, "%s\n", b)
}
