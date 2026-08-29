//go:build linux

package serialenum

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"golang.org/x/sys/unix"
)

func TestListSyntheticFilesystem(t *testing.T) {
	root := t.TempDir()
	oldSysClassTTYDir, oldDevDir := sysClassTTYDir, devDir
	sysClassTTYDir = filepath.Join(root, "sys", "class", "tty")
	devDir = filepath.Join(root, "dev")
	t.Cleanup(func() {
		sysClassTTYDir = oldSysClassTTYDir
		devDir = oldDevDir
	})
	mkdir(t, sysClassTTYDir)
	mkdir(t, devDir)

	platformPort := filepath.Join(root, "sys", "devices", "platform", "serial8250", "tty", "ttyS0")
	makePort(t, "ttyS0", platformPort)
	writeFile(t, filepath.Join(platformPort, "type"), "1\n")

	missingNodePort := filepath.Join(root, "sys", "devices", "platform", "serial8250", "tty", "ttyS1")
	makePort(t, "ttyS1", missingNodePort)
	writeFile(t, filepath.Join(missingNodePort, "type"), "1\n")

	platformAliasPort := filepath.Join(root, "sys", "devices", "platform", "soc", "serial", "tty", "ttyAMA0")
	makePort(t, "ttyAMA0", platformAliasPort)

	phantomPort := filepath.Join(root, "sys", "devices", "platform", "serial8250", "tty", "ttyS2")
	makePort(t, "ttyS2", phantomPort)
	writeFile(t, filepath.Join(phantomPort, "type"), "0\n")

	virtualPort := filepath.Join(root, "sys", "devices", "virtual", "tty", "tty0")
	makePort(t, "tty0", virtualPort)

	rfcommPort := filepath.Join(root, "sys", "devices", "virtual", "tty", "rfcomm0")
	makePort(t, "rfcomm0", rfcommPort)

	usbDevice := filepath.Join(root, "sys", "devices", "pci0000:00", "usb1", "1-1")
	acmInterface := filepath.Join(usbDevice, "1-1:1.0")
	acmPort := filepath.Join(acmInterface, "tty", "ttyACM0")
	makePort(t, "ttyACM0", acmPort)
	makeUSBDevice(t, usbDevice, "1546", "01A9", "u-blox GNSS receiver", "")
	makeUSBInterface(t, acmInterface, "00")

	multiportDevice := filepath.Join(root, "sys", "devices", "pci0000:00", "usb1", "1-9")
	makeUSBDevice(t, multiportDevice, "152a", "8231", "Septentrio USB Device", "0100019577")
	for _, iface := range []struct{ num, tty string }{{"00", "ttyACM1"}, {"02", "ttyACM2"}} {
		dir := filepath.Join(multiportDevice, "1-9:1."+iface.num[1:])
		makePort(t, iface.tty, filepath.Join(dir, "tty", iface.tty))
		makeUSBInterface(t, dir, iface.num)
	}

	usbSerialDevice := filepath.Join(root, "sys", "devices", "pci0000:00", "usb2", "2-1")
	usbSerialInterface := filepath.Join(usbSerialDevice, "2-1:1.0")
	usbSerialPort := filepath.Join(usbSerialInterface, "ttyUSB0", "tty", "ttyUSB0")
	makePort(t, "ttyUSB0", usbSerialPort)
	makeUSBDevice(t, usbSerialDevice, "0403", "6001", "FT232R USB UART", "BG02DBNX")
	makeUSBInterface(t, usbSerialInterface, "00")

	for _, name := range []string{"rfcomm0", "ttyS0", "ttyS2", "ttyAMA0", "ttyACM0", "ttyACM1", "ttyACM2", "ttyUSB0"} {
		writeFile(t, filepath.Join(devDir, name), "")
	}
	makeLink(t, filepath.Join(devDir, "gps0"), "ttyACM0")
	makeLink(t, filepath.Join(devDir, "serial0"), "ttyAMA0")
	makeLink(t, filepath.Join(devDir, "zz-gps"), filepath.Join(devDir, "ttyACM0"))
	makeLink(t, filepath.Join(devDir, "stdin"), "/proc/self/fd/0")
	makeLink(t, filepath.Join(devDir, "nested"), filepath.Join("serial", "by-id", "receiver"))
	mkdir(t, filepath.Join(devDir, "serial", "by-id"))
	makeLink(t, filepath.Join(devDir, "serial", "by-id", "receiver"), filepath.Join("..", "..", "ttyUSB0"))

	got, err := List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	want := []Port{
		{
			Device:  filepath.Join(devDir, "rfcomm0"),
			Display: filepath.Join(devDir, "rfcomm0"),
		},
		{
			Device: filepath.Join(devDir, "ttyACM0"),
			Display: filepath.Join(devDir, "ttyACM0") + " (" +
				filepath.Join(devDir, "gps0") + ", " +
				filepath.Join(devDir, "zz-gps") + ", u-blox gen 9)",
			USB: USBID{
				VID: 0x1546,
				PID: 0x01a9,
			},
			Interface: "00",
			Aliases:   []string{filepath.Join(devDir, "gps0"), filepath.Join(devDir, "zz-gps")},
		},
		{
			Device:    filepath.Join(devDir, "ttyACM1"),
			Display:   filepath.Join(devDir, "ttyACM1") + " (Septentrio USB Device, if00)",
			USB:       USBID{VID: 0x152a, PID: 0x8231},
			Serial:    "0100019577",
			Interface: "00",
		},
		{
			Device:    filepath.Join(devDir, "ttyACM2"),
			Display:   filepath.Join(devDir, "ttyACM2") + " (Septentrio USB Device, if02)",
			USB:       USBID{VID: 0x152a, PID: 0x8231},
			Serial:    "0100019577",
			Interface: "02",
		},
		{
			Device:  filepath.Join(devDir, "ttyAMA0"),
			Display: filepath.Join(devDir, "ttyAMA0") + " (" + filepath.Join(devDir, "serial0") + ")",
			Aliases: []string{filepath.Join(devDir, "serial0")},
		},
		{
			Device:  filepath.Join(devDir, "ttyS0"),
			Display: filepath.Join(devDir, "ttyS0"),
		},
		{
			Device:  filepath.Join(devDir, "ttyUSB0"),
			Display: filepath.Join(devDir, "ttyUSB0") + " (FT232R USB UART)",
			USB: USBID{
				VID: 0x0403,
				PID: 0x6001,
			},
			Serial:    "BG02DBNX",
			Interface: "00",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("List() mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestIsRFCOMMName(t *testing.T) {
	for _, test := range []struct {
		name string
		want bool
	}{
		{"rfcomm0", true},
		{"rfcomm255", true},
		{"rfcomm", false},
		{"rfcommX", false},
		{"tty0", false},
	} {
		if got := isRFCOMMName(test.name); got != test.want {
			t.Errorf("isRFCOMMName(%q) = %v, want %v", test.name, got, test.want)
		}
	}
}

func TestIsDeviceGone(t *testing.T) {
	if !isDeviceGone(os.ErrNotExist) {
		t.Error("isDeviceGone(os.ErrNotExist) = false")
	}
	if !isDeviceGone(unix.ENODEV) {
		t.Error("isDeviceGone(unix.ENODEV) = false")
	}
	if isDeviceGone(unix.EIO) {
		t.Error("isDeviceGone(unix.EIO) = true")
	}
}

func makePort(t *testing.T, name, portPath string) {
	t.Helper()
	mkdir(t, portPath)
	target, err := filepath.Rel(sysClassTTYDir, portPath)
	if err != nil {
		t.Fatal(err)
	}
	makeLink(t, filepath.Join(sysClassTTYDir, name), target)
}

// makeUSBDevice writes the sysfs attributes of a USB device. An empty serial
// stands for a device that publishes no serial number, so the attribute is
// absent rather than empty.
func makeUSBDevice(t *testing.T, dir, vid, pid, product, serial string) {
	t.Helper()
	mkdir(t, dir)
	sysDir := filepath.Dir(filepath.Dir(sysClassTTYDir))
	makeLink(t, filepath.Join(dir, "subsystem"), filepath.Join(sysDir, "bus", "usb"))
	writeFile(t, filepath.Join(dir, "uevent"), "MAJOR=189\nDEVTYPE=usb_device\n")
	writeFile(t, filepath.Join(dir, "idVendor"), vid+"\n")
	writeFile(t, filepath.Join(dir, "idProduct"), pid+"\n")
	writeFile(t, filepath.Join(dir, "product"), product+"\n")
	if serial != "" {
		writeFile(t, filepath.Join(dir, "serial"), serial+"\n")
	}
}

func makeUSBInterface(t *testing.T, dir, num string) {
	t.Helper()
	mkdir(t, dir)
	sysDir := filepath.Dir(filepath.Dir(sysClassTTYDir))
	makeLink(t, filepath.Join(dir, "subsystem"), filepath.Join(sysDir, "bus", "usb"))
	writeFile(t, filepath.Join(dir, "uevent"), "DEVTYPE=usb_interface\n")
	writeFile(t, filepath.Join(dir, "bInterfaceNumber"), num+"\n")
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func makeLink(t *testing.T, path, target string) {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}
