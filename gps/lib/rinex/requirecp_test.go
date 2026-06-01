package rinex

import (
	"reflect"
	"testing"

	"github.com/jclark/satpulse/gps/lib/opt"
)

func TestRequireCPFilterDropsObservationsWithoutPhase(t *testing.T) {
	in := []SignalObservation{
		{Sat: "E11", Sig: "1C", SignalValues: SignalValues{PR: opt.Make(1.0), CP: opt.Make(10.0)}},
		{Sat: "E11", Sig: "5Q", SignalValues: SignalValues{PR: opt.Make(2.0)}},
		{Sat: "G03", Sig: "1C", SignalValues: SignalValues{CP: opt.Make(20.0)}},
	}
	dst := &recordSink{}
	sink := NewRequireCPFilter(dst)
	meta := Metadata{}
	meta.Marker.Name = "MARK"
	if err := sink.Metadata(meta); err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	for _, o := range in {
		if err := sink.Observation(o); err != nil {
			t.Fatalf("Observation: %v", err)
		}
	}
	if err := sink.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	expect := []SignalObservation{in[0], in[2]}
	if !reflect.DeepEqual(dst.obs, expect) {
		t.Errorf("got  %+v\nwant %+v", dst.obs, expect)
	}
	if len(dst.meta) != 1 || dst.meta[0].Marker.Name != "MARK" {
		t.Errorf("metadata = %#v", dst.meta)
	}
	if !dst.flushed {
		t.Error("Flush did not reach wrapped sink")
	}
}
