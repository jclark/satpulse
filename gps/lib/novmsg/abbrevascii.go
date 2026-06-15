package novmsg

import "strings"

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
	for i < len(s) && isAlnum(s[i]) {
		i++
	}
	return &AbbrevAsciiLine{Name: s[:i], Fields: strings.Fields(s[i:])}
}

func isAlnum(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}
