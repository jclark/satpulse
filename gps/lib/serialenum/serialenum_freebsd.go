//go:build freebsd

package serialenum

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"
)

var devDir = "/dev"

// callOutName matches a call-out device node: cuauN for uart(4) ports, or
// cuaUN with an optional .M subunit for multi-port devices on ucom(4). The
// .init and .lock state nodes do not match.
var callOutName = regexp.MustCompile(`^cua(u[0-9]+|U[0-9]+(\.[0-9]+)?)$`)

// usbMeta is the USB identity a ucom(4) driver instance publishes for its
// ports, keyed in the metadata map by its ttyname sysctl value ("U0").
type usbMeta struct {
	usb     USBID
	serial  string
	product string
}

// List enumerates serial ports by scanning /dev for uart(4) and ucom(4)
// call-out devices. USB identity comes from the dev. sysctl tree; a port
// whose driver publishes no metadata there is still listed, just without it.
func List() ([]Port, error) {
	entries, err := os.ReadDir(devDir)
	if err != nil {
		return nil, fmt.Errorf("enumerating serial ports: reading %s: %w", devDir, err)
	}
	var names []string
	for _, entry := range entries {
		if callOutName.MatchString(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	meta, err := ucomMetadata()
	if err != nil {
		return nil, fmt.Errorf("enumerating serial ports: %w", err)
	}
	return buildPorts(names, meta), nil
}

// ucomMetadata walks the dev. sysctl subtree for driver instances that have a
// ttyname node, which the ucom core creates for every USB serial driver
// (ucom_set_pnpinfo_usb in sys/dev/usb/serial/usb_serial.c). The walk is not
// a snapshot, so a node that vanishes mid-walk (device detached) is skipped.
func ucomMetadata() (map[string]usbMeta, error) {
	root, err := name2oid("dev")
	if err != nil {
		return nil, fmt.Errorf("resolving sysctl node dev: %w", err)
	}
	meta := make(map[string]usbMeta)
	for oid := root; ; {
		oid, err = nextOid(oid)
		if err != nil {
			if errors.Is(err, unix.ENOENT) {
				break
			}
			return nil, fmt.Errorf("walking sysctl dev tree: %w", err)
		}
		if len(oid) <= len(root) || !slices.Equal(oid[:len(root)], root) {
			break
		}
		name, err := oidName(oid)
		if err != nil {
			if errors.Is(err, unix.ENOENT) {
				continue
			}
			return nil, fmt.Errorf("naming sysctl node: %w", err)
		}
		node, ok := strings.CutSuffix(name, ".ttyname")
		if !ok {
			continue
		}
		tty, err := sysctlOptional(name)
		if err != nil {
			return nil, fmt.Errorf("reading sysctl %s: %w", name, err)
		}
		if tty == "" {
			continue
		}
		pnpinfo, err := sysctlOptional(node + ".%pnpinfo")
		if err != nil {
			return nil, fmt.Errorf("reading sysctl %s.%%pnpinfo: %w", node, err)
		}
		desc, err := sysctlOptional(node + ".%desc")
		if err != nil {
			return nil, fmt.Errorf("reading sysctl %s.%%desc: %w", node, err)
		}
		vals := parsePnpinfo(pnpinfo)
		meta[tty] = usbMeta{
			usb:     pnpinfoUSBID(vals),
			serial:  vals["sernum"],
			product: descProduct(desc),
		}
	}
	return meta, nil
}

// sysctlOptional reads a string sysctl, treating a node that vanished because
// its device detached mid-walk as absent rather than an error.
func sysctlOptional(name string) (string, error) {
	v, err := unix.Sysctl(name)
	if errors.Is(err, unix.ENOENT) {
		return "", nil
	}
	return v, err
}

// pnpinfoField matches one key=value field of a %pnpinfo string. A value may
// be double-quoted to include spaces; pnpinfo defines no escape syntax, so
// the quotes are plain delimiters, and a truncated value may lack the
// closing one.
var pnpinfoField = regexp.MustCompile(`([^ =]+)=("([^"]*)"?|[^ ]*)`)

// parsePnpinfo splits a %pnpinfo string into its key=value fields.
func parsePnpinfo(s string) map[string]string {
	m := make(map[string]string)
	for _, f := range pnpinfoField.FindAllStringSubmatch(s, -1) {
		val := f[2]
		if strings.HasPrefix(val, `"`) {
			val = f[3]
		}
		m[f[1]] = val
	}
	return m
}

// pnpinfoUSBID converts the vendor= and product= fields, which pnpinfo
// formats as 0x-prefixed hex.
func pnpinfoUSBID(vals map[string]string) USBID {
	return parseEnumeratorUSBID(strings.TrimPrefix(vals["vendor"], "0x"),
		strings.TrimPrefix(vals["product"], "0x"))
}

// descSuffix matches the trailing address details that usb_devinfo appends to
// the string-descriptor text in a USB device's %desc.
var descSuffix = regexp.MustCompile(`, class [0-9]+/[0-9]+, rev [0-9a-f.]+/[0-9a-f.]+, addr [0-9]+$`)

// descProduct extracts best-effort product display text from a %desc value.
// A device that publishes no string descriptors gets numeric
// "vendor 0x... product 0x..." fallback text, which is no use for display.
func descProduct(desc string) string {
	s := descSuffix.ReplaceAllString(desc, "")
	if strings.HasPrefix(s, "vendor 0x") {
		return ""
	}
	return s
}

// buildPorts joins the /dev scan to the ucom metadata. A cuaU port matches
// the instance whose ttyname equals its basename after "cua" or, for the .M
// subunit of a multi-port device, that basename with the subunit stripped.
// ttyname always reports the U unit allocation, so a driver that names its
// ttys with a ucom_tty_name callback simply gets no metadata.
func buildPorts(names []string, meta map[string]usbMeta) []Port {
	ports := make([]Port, 0, len(names))
	for _, name := range names {
		tty := strings.TrimPrefix(name, "cua")
		m, ok := meta[tty]
		if !ok {
			if i := strings.IndexByte(tty, '.'); i >= 0 {
				m = meta[tty[:i]]
			}
		}
		device := filepath.Join(devDir, name)
		ports = append(ports, Port{
			Device:  device,
			Display: enumeratorDisplay(device, m.product, m.usb.VID, m.usb.PID),
			USB:     m.usb,
			Serial:  m.serial,
		})
	}
	return ports
}

// The CTL_SYSCTL_* meta sysctl operations (constants generated from
// sys/sysctl.h into ztypes_freebsd.go): NAME2OID resolves a dotted name to an
// OID, and NEXT/NAME step through the OID tree and name its nodes. x/sys/unix
// reaches these only through by-name helpers that re-resolve the name on
// every call, so issue the raw syscall directly.
//go:generate sh -c "{ echo '//go:build freebsd'; echo; go tool cgo -godefs types_freebsd.go; } | gofmt > ztypes_freebsd.go && rm -rf _obj _cgo_*.o"

func name2oid(name string) ([]int32, error) {
	b, err := unix.ByteSliceFromString(name)
	if err != nil {
		return nil, err
	}
	// Two words of headroom beyond CTL_MAXNAME, matching the kernel's own
	// userland wrapper (see the note in x/sys/unix nametomib).
	oid := make([]int32, unix.CTL_MAXNAME+2)
	n := uintptr(unix.CTL_MAXNAME * 4)
	mib := []int32{ctlSysctl, ctlSysctlName2oid}
	if err := sysctlMib(mib, unsafe.Pointer(&oid[0]), &n, unsafe.Pointer(&b[0]), uintptr(len(name))); err != nil {
		return nil, err
	}
	return oid[:n/4], nil
}

func nextOid(oid []int32) ([]int32, error) {
	next := make([]int32, unix.CTL_MAXNAME+2)
	n := uintptr(unix.CTL_MAXNAME * 4)
	mib := append([]int32{ctlSysctl, ctlSysctlNext}, oid...)
	if err := sysctlMib(mib, unsafe.Pointer(&next[0]), &n, nil, 0); err != nil {
		return nil, err
	}
	return next[:n/4], nil
}

func oidName(oid []int32) (string, error) {
	buf := make([]byte, 256)
	n := uintptr(len(buf))
	mib := append([]int32{ctlSysctl, ctlSysctlName}, oid...)
	if err := sysctlMib(mib, unsafe.Pointer(&buf[0]), &n, nil, 0); err != nil {
		return "", err
	}
	return unix.ByteSliceToString(buf[:n]), nil
}

func sysctlMib(mib []int32, old unsafe.Pointer, oldlen *uintptr, new unsafe.Pointer, newlen uintptr) error {
	_, _, errno := unix.Syscall6(unix.SYS___SYSCTL,
		uintptr(unsafe.Pointer(&mib[0])), uintptr(len(mib)),
		uintptr(old), uintptr(unsafe.Pointer(oldlen)),
		uintptr(new), newlen)
	if errno != 0 {
		return errno
	}
	return nil
}
