package sdpcmd

import (
	"io/fs"
	"reflect"
	"sort"
	"testing"
	"testing/fstest"
)

func TestGetInterfaceInfo(t *testing.T) {
	tests := []struct {
		name      string
		fsys      fs.FS
		ifname    string
		phcIndex  int
		expect    *InterfaceInfo
		expectErr bool
	}{
		{
			name: "interface with 4 pins",
			fsys: fstest.MapFS{
				"class/ptp/ptp0/n_pins": {Data: []byte("4\n")},
				"class/ptp/ptp0/n_external_timestamps": {Data: []byte("4\n")},
				"class/ptp/ptp0/n_periodic_outputs": {Data: []byte("4\n")},
				"class/ptp/ptp0/pins/SDP0": {Data: []byte("")},
				"class/ptp/ptp0/pins/SDP1": {Data: []byte("")},
				"class/ptp/ptp0/pins/SDP2": {Data: []byte("")},
				"class/ptp/ptp0/pins/SDP3": {Data: []byte("")},
			},
			ifname:   "eth0",
			phcIndex: 0,
			expect: &InterfaceInfo{
				Name:              "eth0",
				ClockIndex:        0,
				Pins:              []string{"SDP0", "SDP1", "SDP2", "SDP3"},
				NumExttsChannels:  4,
				NumPeroutChannels: 4,
			},
		},
		{
			name: "n_programmable_pins takes precedence over n_pins",
			fsys: fstest.MapFS{
				"class/ptp/ptp1/n_programmable_pins": {Data: []byte("2\n")},
				"class/ptp/ptp1/n_pins": {Data: []byte("4\n")}, // should be ignored
				"class/ptp/ptp1/n_external_timestamps": {Data: []byte("2\n")},
				"class/ptp/ptp1/n_periodic_outputs": {Data: []byte("2\n")},
				"class/ptp/ptp1/pins/SDP0": {Data: []byte("")},
				"class/ptp/ptp1/pins/SDP1": {Data: []byte("")},
			},
			ifname:   "eth1",
			phcIndex: 1,
			expect: &InterfaceInfo{
				Name:              "eth1",
				ClockIndex:        1,
				Pins:              []string{"SDP0", "SDP1"},
				NumExttsChannels:  2,
				NumPeroutChannels: 2,
			},
		},
		{
			name: "raspberry pi cm4 with single pin",
			fsys: fstest.MapFS{
				"class/ptp/ptp2/n_programmable_pins": {Data: []byte("1\n")},
				"class/ptp/ptp2/n_external_timestamps": {Data: []byte("9\n")},
				"class/ptp/ptp2/n_periodic_outputs": {Data: []byte("1\n")},
				"class/ptp/ptp2/pins/SYNC_OUT": {Data: []byte("")},
			},
			ifname:   "eth0",
			phcIndex: 2,
			expect: &InterfaceInfo{
				Name:              "eth0",
				ClockIndex:        2,
				Pins:              []string{"SYNC_OUT"},
				NumExttsChannels:  9,
				NumPeroutChannels: 1,
			},
		},
		{
			name: "pin count mismatch - returns info AND error",
			fsys: fstest.MapFS{
				"class/ptp/ptp0/n_pins": {Data: []byte("4\n")},
				"class/ptp/ptp0/n_external_timestamps": {Data: []byte("4\n")},
				"class/ptp/ptp0/n_periodic_outputs": {Data: []byte("4\n")},
				"class/ptp/ptp0/pins/SDP0": {Data: []byte("")},
				"class/ptp/ptp0/pins/SDP1": {Data: []byte("")},
				// Only 2 pins instead of 4
			},
			ifname:   "eth0",
			phcIndex: 0,
			expect: &InterfaceInfo{
				Name:              "eth0",
				ClockIndex:        0,
				Pins:              []string{"SDP0", "SDP1"},
				NumExttsChannels:  4,
				NumPeroutChannels: 4,
			},
			expectErr: true,
		},
		{
			name: "no pins - returns nil, nil",
			fsys: fstest.MapFS{
				"class/ptp/ptp0/n_pins": {Data: []byte("0\n")},
				"class/ptp/ptp0/n_external_timestamps": {Data: []byte("0\n")},
				"class/ptp/ptp0/n_periodic_outputs": {Data: []byte("0\n")},
			},
			ifname:   "eth0",
			phcIndex: 0,
			expect:     nil,
		},
		{
			name: "missing n_pins file - returns nil, nil",
			fsys: fstest.MapFS{
				"class/ptp/ptp0/n_external_timestamps": {Data: []byte("4\n")},
				"class/ptp/ptp0/n_periodic_outputs": {Data: []byte("4\n")},
				// n_pins and n_programmable_pins missing
			},
			ifname:   "eth0",
			phcIndex: 0,
			expect:     nil,
		},
		{
			name: "pins directory doesn't exist but n_pins > 0 - error",
			fsys: fstest.MapFS{
				"class/ptp/ptp0/n_pins": {Data: []byte("4\n")},
				"class/ptp/ptp0/n_external_timestamps": {Data: []byte("4\n")},
				"class/ptp/ptp0/n_periodic_outputs": {Data: []byte("4\n")},
				// pins directory missing
			},
			ifname:   "eth0",
			phcIndex: 0,
			expect: &InterfaceInfo{
				Name:              "eth0",
				ClockIndex:        0,
				Pins:              nil,
				NumExttsChannels:  4,
				NumPeroutChannels: 4,
			},
			expectErr: true,
		},
		{
			name: "invalid numbers in sysfs files",
			fsys: fstest.MapFS{
				"class/ptp/ptp0/n_pins": {Data: []byte("not_a_number\n")},
				"class/ptp/ptp0/n_external_timestamps": {Data: []byte("abc\n")},
				"class/ptp/ptp0/n_periodic_outputs": {Data: []byte("def\n")},
			},
			ifname:   "eth0",
			phcIndex: 0,
			expect:     nil, // n_pins is 0 because parse failed
		},
		{
			name: "whitespace handling in sysfs files",
			fsys: fstest.MapFS{
				"class/ptp/ptp0/n_pins": {Data: []byte("  2  \n")},
				"class/ptp/ptp0/n_external_timestamps": {Data: []byte("\t3\t\n")},
				"class/ptp/ptp0/n_periodic_outputs": {Data: []byte(" 4 ")},
				"class/ptp/ptp0/pins/SDP0": {Data: []byte("")},
				"class/ptp/ptp0/pins/SDP1": {Data: []byte("")},
			},
			ifname:   "eth0",
			phcIndex: 0,
			expect: &InterfaceInfo{
				Name:              "eth0",
				ClockIndex:        0,
				Pins:              []string{"SDP0", "SDP1"},
				NumExttsChannels:  3,
				NumPeroutChannels: 4,
			},
		},
		{
			name: "subdirectories in pins should be ignored",
			fsys: fstest.MapFS{
				"class/ptp/ptp0/n_pins": {Data: []byte("2\n")},
				"class/ptp/ptp0/n_external_timestamps": {Data: []byte("2\n")},
				"class/ptp/ptp0/n_periodic_outputs": {Data: []byte("2\n")},
				"class/ptp/ptp0/pins/SDP0": {Data: []byte("")},
				"class/ptp/ptp0/pins/SDP1": {Data: []byte("")},
				"class/ptp/ptp0/pins/subdir/file": {Data: []byte("")}, // This creates a subdir
			},
			ifname:   "eth0",
			phcIndex: 0,
			expect: &InterfaceInfo{
				Name:              "eth0",
				ClockIndex:        0,
				Pins:              []string{"SDP0", "SDP1"},
				NumExttsChannels:  2,
				NumPeroutChannels: 2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getInterfaceInfo(tt.fsys, tt.ifname, tt.phcIndex)

			// Check error
			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// Check result
			if !reflect.DeepEqual(got, tt.expect) {
				t.Errorf("getInterfaceInfo() = %+v, expect %+v", got, tt.expect)
			}
		})
	}
}

func TestReadSysfsInt(t *testing.T) {
	tests := []struct {
		name   string
		fsys   fs.FS
		path   string
		expect int
	}{
		{
			name: "valid integer",
			fsys: fstest.MapFS{
				"test/file": {Data: []byte("42\n")},
			},
			path: "test/file",
			expect: 42,
		},
		{
			name: "integer with whitespace",
			fsys: fstest.MapFS{
				"test/file": {Data: []byte("  123  \n")},
			},
			path: "test/file",
			expect: 123,
		},
		{
			name: "file doesn't exist",
			fsys: fstest.MapFS{},
			path: "test/nonexistent",
			expect: 0,
		},
		{
			name: "invalid integer",
			fsys: fstest.MapFS{
				"test/file": {Data: []byte("not_a_number\n")},
			},
			path: "test/file",
			expect: 0,
		},
		{
			name: "empty file",
			fsys: fstest.MapFS{
				"test/file": {Data: []byte("")},
			},
			path: "test/file",
			expect: 0,
		},
		{
			name: "negative integer",
			fsys: fstest.MapFS{
				"test/file": {Data: []byte("-5\n")},
			},
			path: "test/file",
			expect: -5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := readSysfsInt(tt.fsys, tt.path)
			if got != tt.expect {
				t.Errorf("readSysfsInt() = %d, expect %d", got, tt.expect)
			}
		})
	}
}

func TestReadPinNames(t *testing.T) {
	tests := []struct {
		name      string
		fsys      fs.FS
		ptpPath   string
		expect    []string
		expectErr bool
	}{
		{
			name: "multiple pin files",
			fsys: fstest.MapFS{
				"ptp0/pins/SDP0": {Data: []byte("")},
				"ptp0/pins/SDP1": {Data: []byte("")},
				"ptp0/pins/SDP2": {Data: []byte("")},
				"ptp0/pins/SDP3": {Data: []byte("")},
			},
			ptpPath: "ptp0",
			expect:    []string{"SDP0", "SDP1", "SDP2", "SDP3"},
		},
		{
			name: "empty pins directory",
			fsys: fstest.MapFS{
				"ptp0/pins/.gitkeep": {Data: []byte("")}, // Just to create the directory
			},
			ptpPath: "ptp0",
			expect:    []string{".gitkeep"},
		},
		{
			name:    "pins directory doesn't exist",
			fsys:    fstest.MapFS{},
			ptpPath: "ptp0",
			expect:    nil,
			expectErr: false, // Returns nil, nil for non-existent directory
		},
		{
			name: "mixed files and subdirectories",
			fsys: fstest.MapFS{
				"ptp0/pins/SDP0": {Data: []byte("")},
				"ptp0/pins/SDP1": {Data: []byte("")},
				"ptp0/pins/subdir/file": {Data: []byte("")}, // Creates a subdir
			},
			ptpPath: "ptp0",
			expect:    []string{"SDP0", "SDP1"},
		},
		{
			name: "various realistic pin names",
			fsys: fstest.MapFS{
				"ptp0/pins/SYNC_OUT": {Data: []byte("")},
				"ptp0/pins/SYNC_IN": {Data: []byte("")},
				"ptp0/pins/SDP0": {Data: []byte("")},
				"ptp0/pins/SDP1": {Data: []byte("")},
			},
			ptpPath: "ptp0",
			expect:    []string{"SYNC_OUT", "SYNC_IN", "SDP0", "SDP1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readPinNames(tt.fsys, tt.ptpPath)
			if (err != nil) != tt.expectErr {
				t.Errorf("readPinNames() error = %v, expectErr %v", err, tt.expectErr)
				return
			}

			// Sort both slices for consistent comparison since ReadDir order may vary
			sort.Strings(got)
			sort.Strings(tt.expect)
			if !reflect.DeepEqual(got, tt.expect) {
				t.Errorf("readPinNames() = %v, expect %v", got, tt.expect)
			}
		})
	}
}