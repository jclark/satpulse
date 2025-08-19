package uncmsg

import "strings"

// Mode represents the response to the MODE query command.
// This message is ASCII-only and uses an 8-bit XOR checksum.
// No binary message ID is assigned.
//
// The message has two comma-separated fields in the payload:
// 1. MODE - Operating mode (e.g., "MODE ROVER SURVEY", "MODE BASE TIME 60 2.5 3.5")
// 2. HEADINGMODE - HEADING2 mode info (empty string if HEADING2 disabled, dual-antenna only)
//
// Example: "#MODE,97,GPS,FINE,2379,562467000,0,0,18,271;MODE BASE 1234 TIME 60 2.5 3.5,*53"
type Mode struct {
	// Mode contains the operating mode string
	// Examples:
	// - "MODE ROVER SURVEY"
	// - "MODE BASE -1144698.0455 6090335.4099 1504171.3914"  
	// - "MODE BASE 1234 TIME 60 2.5 3.5"
	Mode string
	
	// HeadingMode contains the HEADING2 mode information (empty if disabled or single-antenna)
	// Examples:
	// - "" (empty for single-antenna receivers like UM980)
	// - "HEADINGMODE FIXLENGTH"
	// - "HEADINGMODE VARIABLELENGTH"
	// - "HEADINGMODE STATIC"
	HeadingMode string
}

// ID returns the message identifiers for Mode
// Returns 0 for MsgID since this is an ASCII-only message
func (m *Mode) ID() (MsgID, string) {
	return 0, "MODE"
}

// Command returns the equivalent MODE command that would establish this mode
func (m *Mode) Command() string {
	cmd := m.Mode
	if cmd == "MODE HEADING2" && strings.HasPrefix(m.HeadingMode, "HEADINGMODE ") {
		cmd += m.HeadingMode[len("HEADINGMODE"):]
	}
	return cmd
}

func init() {
	// Register Mode message type
	// Note: Mode has no binary message ID, only ASCII format
	regMsg[Mode]("MODE")
}