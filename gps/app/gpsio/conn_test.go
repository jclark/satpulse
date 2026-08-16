package gpsio

import "testing"

func TestPPSMethodText(t *testing.T) {
	tests := []struct {
		method PPSMethod
		text   string
	}{
		{PPSMethodPoll, "poll"},
		{PPSMethodWait, "wait"},
	}
	for _, tc := range tests {
		t.Run(tc.text, func(t *testing.T) {
			if got := tc.method.String(); got != tc.text {
				t.Errorf("String() = %q, want %q", got, tc.text)
			}
			got, err := ParsePPSMethod(tc.text)
			if err != nil || got != tc.method {
				t.Errorf("ParsePPSMethod(%q) = %v, %v; want %v, nil", tc.text, got, err, tc.method)
			}
			text, err := tc.method.MarshalText()
			if err != nil || string(text) != tc.text {
				t.Errorf("MarshalText() = %q, %v; want %q, nil", text, err, tc.text)
			}
			var decoded PPSMethod
			if err := decoded.UnmarshalText([]byte(tc.text)); err != nil || decoded != tc.method {
				t.Errorf("UnmarshalText(%q) produced %v, %v; want %v, nil", tc.text, decoded, err, tc.method)
			}
		})
	}
}

func TestPPSMethodInvalidText(t *testing.T) {
	for _, text := range []string{"", "auto", "kernel", "Poll"} {
		t.Run(text, func(t *testing.T) {
			if _, err := ParsePPSMethod(text); err == nil {
				t.Errorf("ParsePPSMethod(%q) succeeded", text)
			}
			method := PPSMethodWait
			if err := method.UnmarshalText([]byte(text)); err == nil {
				t.Errorf("UnmarshalText(%q) succeeded", text)
			}
			if method != PPSMethodWait {
				t.Errorf("failed UnmarshalText changed method to %v", method)
			}
		})
	}
}

func TestPPSMethodWithoutTextForm(t *testing.T) {
	for _, method := range []PPSMethod{-1, 0, PPSMethodWait + 1} {
		t.Run(method.String(), func(t *testing.T) {
			if _, err := method.MarshalText(); err == nil {
				t.Errorf("PPSMethod(%d).MarshalText succeeded", method)
			}
		})
	}
}
