package novmsg

import (
	"reflect"
	"testing"
)

func TestParseAbbrevAsciiLine(t *testing.T) {
	tests := []struct {
		name   string
		packet string
		expect *AbbrevAsciiLine
	}{
		{
			"LOGLIST header line",
			"<LOGLIST COM3 17548 95.000000 FINE 2413 3196.000000 42155794 830 18\r\n",
			&AbbrevAsciiLine{
				Name:   "LOGLIST",
				Fields: []string{"COM3", "17548", "95.000000", "FINE", "2413", "3196.000000", "42155794", "830", "18"},
			},
		},
		{
			"TAB count line",
			"<\t1\r\n",
			&AbbrevAsciiLine{Name: "", Fields: []string{"1"}},
		},
		{
			"TAB log entry line",
			"<\tRECTIMEB COM3 1 \r\n",
			&AbbrevAsciiLine{Name: "", Fields: []string{"RECTIMEB", "COM3", "1"}},
		},
		{
			"space continuation line",
			"<     32\r\n",
			&AbbrevAsciiLine{Name: "", Fields: []string{"32"}},
		},
		{
			"OK response",
			"<OK\r\n",
			&AbbrevAsciiLine{Name: "OK", Fields: []string{}},
		},
		{
			"ERROR response",
			"<ERROR:Invalid Message ID\r\n",
			&AbbrevAsciiLine{Name: "ERROR", Fields: []string{":Invalid", "Message", "ID"}},
		},
		{
			"empty line",
			"<\r\n",
			&AbbrevAsciiLine{Name: "", Fields: []string{}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseAbbrevAsciiLine([]byte(tt.packet))
			if !reflect.DeepEqual(got, tt.expect) {
				t.Errorf("got  %+v\nwant %+v", got, tt.expect)
			}
		})
	}
}
