package septentrio

import (
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
)

// The test packets are verbatim captures from a mosaic-G5 (v1.1.0).
func TestParseReply(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		expect    *Reply
		expectErr bool
	}{
		{
			name: "get ack with one state line",
			data: "$R: getCalibCommonDelay\r\n  CalibCommonDelay, 0.000\r\nUSB1>",
			expect: &Reply{
				Kind:   ReplyAck,
				Echo:   "getCalibCommonDelay",
				States: []string{"CalibCommonDelay, 0.000"},
				Prompt: "USB1",
			},
		},
		{
			name: "set ack echoes command and achieved value",
			data: "$R: setPPSParameters, , , , Galileo\r\n  PPSParameters, sec1, Low2High, 0.00, Galileo, 1, 100.000000\r\nUSB1>",
			expect: &Reply{
				Kind:   ReplyAck,
				Echo:   "setPPSParameters, , , , Galileo",
				States: []string{"PPSParameters, sec1, Low2High, 0.00, Galileo, 1, 100.000000"},
				Prompt: "USB1",
			},
		},
		{
			name: "ack with two state lines",
			data: "$R: getElevationMask, all\r\n  ElevationMask, Tracking, 5\r\n  ElevationMask, PVT, 5\r\nUSB1>",
			expect: &Reply{
				Kind:   ReplyAck,
				Echo:   "getElevationMask, all",
				States: []string{"ElevationMask, Tracking, 5", "ElevationMask, PVT, 5"},
				Prompt: "USB1",
			},
		},
		{
			name: "ack with no state lines",
			data: "$R: setSBFOutput, Stream9, none, none, off\r\nUSB1>",
			expect: &Reply{
				Kind:   ReplyAck,
				Echo:   "setSBFOutput, Stream9, none, none, off",
				Prompt: "USB1",
			},
		},
		{
			name: "nak with error text",
			data: "$R? setSignalTracking: Argument 'Signal' is invalid!\r\nUSB1>",
			expect: &Reply{
				Kind:   ReplyNak,
				Error:  "setSignalTracking: Argument 'Signal' is invalid!",
				Prompt: "USB1",
			},
		},
		{
			name: "lst open ends at pseudo-prompt",
			data: "$R; lstInternalFile, Identification\r\n---->",
			expect: &Reply{
				Kind: ReplyLst,
				Echo: "lstInternalFile, Identification",
			},
		},
		{
			name: "lst open with blank line",
			data: "$R; lstConfigFile, Boot\r\n\r\n---->",
			expect: &Reply{
				Kind: ReplyLst,
				Echo: "lstConfigFile, Boot",
			},
		},
		{
			name: "intermediate block unit",
			data: "$-- BLOCK 1 / 5\r\n<?xml version=\"1.0\" encoding=\"ISO-8859-1\" ?>\r\n<rxproduct xmlns=\"http://septentrio.com/ns/ProductDescription/2.9\">\r\n\r\n---->",
			expect: &Reply{
				Kind:       ReplyBlock,
				Block:      "<?xml version=\"1.0\" encoding=\"ISO-8859-1\" ?>\n<rxproduct xmlns=\"http://septentrio.com/ns/ProductDescription/2.9\">\n",
				BlockNum:   1,
				BlockTotal: 5,
			},
		},
		{
			name: "final block unit ends at real prompt",
			data: "$-- BLOCK 5 / 5\r\n</rxproduct>\r\n\r\nUSB1>",
			expect: &Reply{
				Kind:       ReplyBlock,
				Block:      "</rxproduct>\n",
				BlockNum:   5,
				BlockTotal: 5,
				Prompt:     "USB1",
			},
		},
		{
			name: "block with unknown total",
			data: "$-- BLOCK 1 / 0\r\n# Configuration File \"Boot\"\r\n# Equal to RxDefault!\r\nUSB1>",
			expect: &Reply{
				Kind:       ReplyBlock,
				Block:      "# Configuration File \"Boot\"\n# Equal to RxDefault!",
				BlockNum:   1,
				BlockTotal: 0,
				Prompt:     "USB1",
			},
		},
		{
			name:      "too short",
			data:      "USB1>",
			expectErr: true,
		},
		{
			name:      "no $R header",
			data:      "$GPGGA,foo\r\nUSB1>",
			expectErr: true,
		},
		{
			name:      "unknown reply type",
			data:      "$R= huh\r\nUSB1>",
			expectErr: true,
		},
		{
			name:      "bad block header",
			data:      "$-- BLOB 1 / 5\r\nx\r\nUSB1>",
			expectErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseReply(tc.data)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", got, tc.expect)
			}
		})
	}
}

type nativeMsgRecorder struct {
	tag   gpsprot.Tag
	msgID string
	msg   any
}

func (r *nativeMsgRecorder) NativeMsg(tag gpsprot.Tag, msgID string, msg any, tRead time.Time) error {
	r.tag, r.msgID, r.msg = tag, msgID, msg
	return nil
}

func TestReplyProcessorProcessPacket(t *testing.T) {
	tests := []struct {
		name        string
		data        string
		expectMsgID string
		expectMsg   *Reply
		expectErr   bool
	}{
		{
			name:        "ack delivered as native message",
			data:        "$R: getCalibCommonDelay\r\n  CalibCommonDelay, 0.000\r\nUSB1>",
			expectMsgID: "ack",
			expectMsg: &Reply{
				Kind:   ReplyAck,
				Echo:   "getCalibCommonDelay",
				States: []string{"CalibCommonDelay, 0.000"},
				Prompt: "USB1",
			},
		},
		{
			name:        "nak delivered as native message",
			data:        "$R? setCalibCommonDelay: Argument 'CommonDelay' is invalid!\r\nUSB1>",
			expectMsgID: "nak",
			expectMsg: &Reply{
				Kind:   ReplyNak,
				Error:  "setCalibCommonDelay: Argument 'CommonDelay' is invalid!",
				Prompt: "USB1",
			},
		},
		{
			name:      "parse error",
			data:      "USB1>",
			expectErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := NewReplyProcessor()
			rec := &nativeMsgRecorder{}
			p.SetNativeMsgHandler(rec)
			msgID, err := p.ProcessPacket(tc.data, time.Time{})
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error, got msgID %q", msgID)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if msgID != tc.expectMsgID {
				t.Errorf("msgID: got %q want %q", msgID, tc.expectMsgID)
			}
			if rec.tag != TagReply {
				t.Errorf("tag: got %q want %q", rec.tag, TagReply)
			}
			if rec.msgID != tc.expectMsgID {
				t.Errorf("handler msgID: got %q want %q", rec.msgID, tc.expectMsgID)
			}
			if !reflect.DeepEqual(rec.msg, tc.expectMsg) {
				t.Errorf("msg: got  %+v\nwant %+v", rec.msg, tc.expectMsg)
			}
		})
	}
}
