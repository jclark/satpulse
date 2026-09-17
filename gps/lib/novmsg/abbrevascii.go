package novmsg

import (
	"strings"

	"github.com/jclark/satpulse/gps/lib/ascii"
)

// AbbrevSync is the byte that starts every NovAtel abbreviated ASCII line.
const AbbrevSync byte = '<'

// AbbrevAsciiLine represents a line of a NovAtel abbreviated ASCII message.
// A message spans one or more lines, each starting with '<': the header
// line starts with a name directly after the '<' (e.g. "LOGLIST" or "OK");
// continuation lines are indented with whitespace and have an empty Name.
type AbbrevAsciiLine struct {
	Name   string   // leading alphanumeric token; empty for continuation lines
	Fields []string // whitespace-separated fields after the name
}

// ParseAbbrevAsciiLine parses one abbreviated ASCII line.
// pkt must be a valid abbreviated ASCII packet: '<' followed by the line
// content and CR/LF.
func ParseAbbrevAsciiLine(pkt []byte) *AbbrevAsciiLine {
	s := strings.TrimSuffix(string(pkt[1:]), "\r\n")
	i := 0
	for i < len(s) && ascii.IsAlnum(s[i]) {
		i++
	}
	return &AbbrevAsciiLine{Name: s[:i], Fields: strings.Fields(s[i:])}
}
