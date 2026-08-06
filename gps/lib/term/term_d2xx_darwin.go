//go:build darwin

// Set FTD2XX_DIR to the directory containing FTDI's ftd2xx.h and WinTypes.h.
//go:generate ./generate_ftd2xx.sh

package term

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"golang.org/x/sys/unix"
)

type cPointer = unsafe.Pointer

type d2xxAPI struct {
	lib                    uintptr
	createDeviceInfoList   func(*uint32) uint32
	getDeviceInfoList      func(cPointer, *uint32) uint32
	openEx                 func(string, uint32, *cPointer) uint32
	close                  func(cPointer) uint32
	setBaudRate            func(cPointer, uint32) uint32
	setDataCharacteristics func(cPointer, uint8, uint8, uint8) uint32
	setFlowControl         func(cPointer, uint16, uint8, uint8) uint32
	setTimeouts            func(cPointer, uint32, uint32) uint32
	read                   func(cPointer, []byte, uint32, *uint32) uint32
	write                  func(cPointer, []byte, uint32, *uint32) uint32
	getStatus              func(cPointer, *uint32, *uint32, *uint32) uint32
	getModemStatus         func(cPointer, *uint32) uint32
	purge                  func(cPointer, uint32) uint32
	setEventNotification   func(cPointer, uint32, cPointer) uint32
	calloc                 func(uintptr, uintptr) cPointer
	free                   func(cPointer)
	pthreadCondInit        func(cPointer, cPointer) int32
	pthreadCondDestroy     func(cPointer) int32
	pthreadCondWait        func(cPointer, cPointer) int32
	pthreadCondBroadcast   func(cPointer) int32
	pthreadMutexInit       func(cPointer, cPointer) int32
	pthreadMutexDestroy    func(cPointer) int32
	pthreadMutexLock       func(cPointer) int32
	pthreadMutexUnlock     func(cPointer) int32
}

// d2xxTerm is a Term implemented on FTDI's D2XX library instead of a tty.
type d2xxTerm struct {
	api    *d2xxAPI
	handle cPointer
	event  *d2xxEvent
	path   string
	attrCache
}

var _ Term = (*d2xxTerm)(nil)
var _ ModemControlLineWaiter = (*d2xxTerm)(nil)

type d2xxEvent struct {
	api  *d2xxAPI
	ptr  cPointer
	wake atomic.Bool
}

// Sizes and offsets of the C structures, taken from the generated Go types so
// that they cannot drift from the layout cgo derived from the headers.
const (
	ftEventHandleSize        = unsafe.Sizeof(ftEventHandle{})
	ftEventCondOffset        = unsafe.Offsetof(ftEventHandle{}.ECondVar)
	ftEventMutexOffset       = unsafe.Offsetof(ftEventHandle{}.EMutex)
	ftDeviceInfoNodeSize     = unsafe.Sizeof(ftDeviceListInfoNode{})
	ftDeviceInfoSerialOffset = unsafe.Offsetof(ftDeviceListInfoNode{}.SerialNumber)
	ftDeviceInfoSerialLen    = len(ftDeviceListInfoNode{}.SerialNumber)
)

var loadD2XX = sync.OnceValues(newD2XXAPI)

// tryD2XX opens path through the D2XX library, reporting false when the path
// does not name an FTDI device that the library can open.
func tryD2XX(path string, opts []AttrSetter) (Term, bool, error) {
	serial, ok := d2xxPathSerial(path)
	if !ok {
		return nil, false, nil
	}
	api, err := loadD2XX()
	if err != nil {
		return nil, false, nil
	}
	serials, err := api.devices(path)
	if err != nil {
		return nil, false, err
	}
	found := false
	for _, s := range serials {
		if s == serial {
			found = true
			break
		}
	}
	if !found {
		return nil, false, nil
	}
	attr := d2xxDefaultAttr()
	for _, opt := range opts {
		if err := opt(&attr); err != nil {
			return nil, false, err
		}
	}
	d, err := api.open(path, serial)
	if err != nil {
		return nil, false, err
	}
	if err := d.setAttr(attr); err != nil {
		d.Close()
		return nil, false, err
	}
	return d, true, nil
}

func newD2XXAPI() (*d2xxAPI, error) {
	lib, err := openD2XXLibrary()
	if err != nil {
		return nil, err
	}
	api := &d2xxAPI{lib: lib}
	for _, fn := range []struct {
		name string
		ptr  any
	}{
		{"FT_CreateDeviceInfoList", &api.createDeviceInfoList},
		{"FT_GetDeviceInfoList", &api.getDeviceInfoList},
		{"FT_OpenEx", &api.openEx},
		{"FT_Close", &api.close},
		{"FT_SetBaudRate", &api.setBaudRate},
		{"FT_SetDataCharacteristics", &api.setDataCharacteristics},
		{"FT_SetFlowControl", &api.setFlowControl},
		{"FT_SetTimeouts", &api.setTimeouts},
		{"FT_Read", &api.read},
		{"FT_Write", &api.write},
		{"FT_GetStatus", &api.getStatus},
		{"FT_GetModemStatus", &api.getModemStatus},
		{"FT_Purge", &api.purge},
		{"FT_SetEventNotification", &api.setEventNotification},
	} {
		if err := registerD2XXFunc(lib, fn.name, fn.ptr); err != nil {
			purego.Dlclose(lib)
			return nil, err
		}
	}
	for _, fn := range []struct {
		name string
		ptr  any
	}{
		{"calloc", &api.calloc},
		{"free", &api.free},
		{"pthread_cond_init", &api.pthreadCondInit},
		{"pthread_cond_destroy", &api.pthreadCondDestroy},
		{"pthread_cond_wait", &api.pthreadCondWait},
		{"pthread_cond_broadcast", &api.pthreadCondBroadcast},
		{"pthread_mutex_init", &api.pthreadMutexInit},
		{"pthread_mutex_destroy", &api.pthreadMutexDestroy},
		{"pthread_mutex_lock", &api.pthreadMutexLock},
		{"pthread_mutex_unlock", &api.pthreadMutexUnlock},
	} {
		if err := registerD2XXFunc(purego.RTLD_DEFAULT, fn.name, fn.ptr); err != nil {
			purego.Dlclose(lib)
			return nil, err
		}
	}
	return api, nil
}

func openD2XXLibrary() (uintptr, error) {
	paths := []string{"libftd2xx.dylib", "/usr/local/lib/libftd2xx.dylib", "/opt/homebrew/lib/libftd2xx.dylib"}
	if path := os.Getenv("SATPULSE_D2XX_LIB"); path != "" {
		paths = append([]string{path}, paths...)
	}
	var errs []error
	for _, path := range paths {
		lib, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err == nil {
			return lib, nil
		}
		errs = append(errs, err)
	}
	return 0, fmt.Errorf("D2XX library is unavailable: %w", errors.Join(errs...))
}

func registerD2XXFunc(lib uintptr, name string, fn any) error {
	sym, err := purego.Dlsym(lib, name)
	if err != nil {
		return fmt.Errorf("D2XX symbol %s is unavailable: %w", name, err)
	}
	purego.RegisterFunc(fn, sym)
	return nil
}

func (api *d2xxAPI) devices(path string) ([]string, error) {
	var n uint32
	if st := api.createDeviceInfoList(&n); st != ftOK {
		return nil, d2xxError(path, "FT_CreateDeviceInfoList", int(st))
	}
	if n == 0 {
		return nil, nil
	}
	buf := api.calloc(uintptr(n), ftDeviceInfoNodeSize)
	if buf == nil {
		return nil, &os.PathError{Op: "allocate D2XX device list", Path: path, Err: errors.New("calloc failed")}
	}
	defer api.free(buf)
	allocated := n
	if st := api.getDeviceInfoList(buf, &n); st != ftOK {
		return nil, d2xxError(path, "FT_GetDeviceInfoList", int(st))
	}
	if n > allocated {
		return nil, &os.PathError{Op: "FT_GetDeviceInfoList", Path: path, Err: errors.New("D2XX device list grew unexpectedly")}
	}
	serials := make([]string, n)
	for i := range n {
		p := addCPointer(buf, uintptr(i)*ftDeviceInfoNodeSize+ftDeviceInfoSerialOffset)
		serials[i] = cString(p, ftDeviceInfoSerialLen)
	}
	return serials, nil
}

func (api *d2xxAPI) open(path, serial string) (*d2xxTerm, error) {
	event, err := newD2XXEvent(api)
	if err != nil {
		return nil, &os.PathError{Op: "create D2XX event", Path: path, Err: err}
	}
	var handle cPointer
	if st := api.openEx(serial, ftOpenBySerialNumber, &handle); st != ftOK {
		event.close()
		return nil, d2xxError(path, "FT_OpenEx", int(st))
	}
	if st := api.setEventNotification(handle, ftEventRXChar|ftEventModemStatus, event.ptr); st != ftOK {
		api.close(handle)
		event.close()
		return nil, d2xxError(path, "FT_SetEventNotification", int(st))
	}
	return &d2xxTerm{api: api, handle: handle, event: event, path: path}, nil
}

func newD2XXEvent(api *d2xxAPI) (*d2xxEvent, error) {
	ptr := api.calloc(1, ftEventHandleSize)
	if ptr == nil {
		return nil, errors.New("calloc failed")
	}
	e := &d2xxEvent{api: api, ptr: ptr}
	if err := api.pthreadCondInit(e.cond(), nil); err != 0 {
		api.free(ptr)
		return nil, fmt.Errorf("pthread_cond_init: %d", err)
	}
	if err := api.pthreadMutexInit(e.mutex(), nil); err != 0 {
		api.pthreadCondDestroy(e.cond())
		api.free(ptr)
		return nil, fmt.Errorf("pthread_mutex_init: %d", err)
	}
	return e, nil
}

func (e *d2xxEvent) wait() error {
	if err := e.api.pthreadMutexLock(e.mutex()); err != 0 {
		return fmt.Errorf("pthread_mutex_lock: %d", err)
	}
	var err int32
	if !e.wake.Load() {
		err = e.api.pthreadCondWait(e.cond(), e.mutex())
	}
	unlockErr := e.api.pthreadMutexUnlock(e.mutex())
	if err != 0 {
		return fmt.Errorf("pthread_cond_wait: %d", err)
	}
	if unlockErr != 0 {
		return fmt.Errorf("pthread_mutex_unlock: %d", unlockErr)
	}
	return nil
}

func (e *d2xxEvent) signal() {
	e.api.pthreadMutexLock(e.mutex())
	e.wake.Store(true)
	e.api.pthreadCondBroadcast(e.cond())
	e.api.pthreadMutexUnlock(e.mutex())
}

func (e *d2xxEvent) close() {
	e.api.pthreadMutexDestroy(e.mutex())
	e.api.pthreadCondDestroy(e.cond())
	e.api.free(e.ptr)
	e.ptr = nil
}

func (e *d2xxEvent) cond() cPointer { return addCPointer(e.ptr, ftEventCondOffset) }

func (e *d2xxEvent) mutex() cPointer { return addCPointer(e.ptr, ftEventMutexOffset) }

func addCPointer(p cPointer, offset uintptr) cPointer { return unsafe.Add(p, offset) }

func cString(p cPointer, size int) string {
	b := unsafe.Slice((*byte)(p), size)
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

func d2xxPathSerial(path string) (string, bool) {
	if p, err := filepath.EvalSymlinks(path); err == nil {
		path = p
	}
	name := filepath.Base(path)
	for _, prefix := range []string{"cu.usbserial-", "tty.usbserial-", "cu.usbserial", "tty.usbserial"} {
		if serial, ok := strings.CutPrefix(name, prefix); ok {
			return serial, serial != ""
		}
	}
	return "", false
}

func d2xxDefaultAttr() Attr {
	var attr Attr
	for _, opt := range []AttrSetter{
		RawMode,
		Local,
		NoParity,
		NoFlowControl,
		ReadTimeout(time.Duration(ftDefaultRXTimeout) * time.Millisecond),
		Speed(9600),
	} {
		if err := opt(&attr); err != nil {
			panic(err)
		}
	}
	return attr
}

// Change changes the attributes of the terminal. The library applies them
// immediately, so unlike a tty there is nothing to drain first.
func (d *d2xxTerm) Change(opts ...AttrSetter) error {
	attr := d.loadAttr()
	for _, opt := range opts {
		if err := opt(&attr); err != nil {
			return err
		}
	}
	return d.setAttr(attr)
}

func (d *d2xxTerm) setAttr(attr Attr) error {
	if st := d.api.setBaudRate(d.handle, uint32(attr.speed())); st != ftOK {
		return d2xxError(d.path, "FT_SetBaudRate", int(st))
	}
	bits := ftBits8
	if attr.ts.Cflag&unix.CSIZE == unix.CS7 {
		bits = ftBits7
	}
	stop := ftStopBits1
	if attr.ts.Cflag&unix.CSTOPB != 0 {
		stop = ftStopBits2
	}
	parity := ftParityNone
	if attr.ts.Cflag&unix.PARENB != 0 {
		parity = ftParityEven
		if attr.ts.Cflag&unix.PARODD != 0 {
			parity = ftParityOdd
		}
	}
	if st := d.api.setDataCharacteristics(d.handle, uint8(bits), uint8(stop), uint8(parity)); st != ftOK {
		return d2xxError(d.path, "FT_SetDataCharacteristics", int(st))
	}
	flow := ftFlowNone
	if attr.ts.Cflag&unix.CRTSCTS != 0 {
		flow = ftFlowRTSCTS
	} else if attr.ts.Iflag&(unix.IXON|unix.IXOFF) != 0 {
		flow = ftFlowXONXOFF
	}
	if st := d.api.setFlowControl(d.handle, uint16(flow), 0, 0); st != ftOK {
		return d2xxError(d.path, "FT_SetFlowControl", int(st))
	}
	readTimeout := uint32(attr.ts.Cc[unix.VTIME]) * uint32(decisecond/time.Millisecond)
	if st := d.api.setTimeouts(d.handle, readTimeout, 0); st != ftOK {
		return d2xxError(d.path, "FT_SetTimeouts", int(st))
	}
	d.storeAttr(attr)
	return nil
}

func (d *d2xxTerm) Path() string { return d.path }

func (d *d2xxTerm) Speed() int {
	attr := d.loadAttr()
	return attr.speed()
}

// DevKind reports DevUSBtoUART: D2XX only opens FTDI bridge chips.
func (d *d2xxTerm) DevKind() DevKind { return DevUSBtoUART }

// TransmitTime returns the time it takes to send nBytes at the current speed.
func (d *d2xxTerm) TransmitTime(nBytes int) time.Duration {
	if nBytes <= 0 {
		return 0
	}
	attr := d.loadAttr()
	return attr.byteTransmitTime() * time.Duration(nBytes)
}

const maxD2XXTransfer = int(^uint32(0))

func (d *d2xxTerm) Read(buf []byte) (int, error) {
	if len(buf) == 0 {
		return 0, nil
	}
	if len(buf) > maxD2XXTransfer {
		buf = buf[:maxD2XXTransfer]
	}
	var n uint32
	st := d.api.read(d.handle, buf, uint32(len(buf)), &n)
	if st != ftOK {
		return int(n), d2xxError(d.path, "FT_Read", int(st))
	}
	if n == 0 {
		attr := d.loadAttr()
		if attr.readCanTimeout() {
			return 0, &os.PathError{Op: "read", Path: d.path, Err: os.ErrDeadlineExceeded}
		}
		return 0, io.EOF
	}
	return int(n), nil
}

func (d *d2xxTerm) Write(buf []byte) (int, error) {
	total := 0
	for len(buf) > 0 {
		size := min(len(buf), maxD2XXTransfer)
		var n uint32
		st := d.api.write(d.handle, buf[:size], uint32(size), &n)
		total += int(n)
		buf = buf[n:]
		if st != ftOK {
			return total, d2xxError(d.path, "FT_Write", int(st))
		}
		if n == 0 {
			return total, io.ErrShortWrite
		}
	}
	return total, nil
}

func (d *d2xxTerm) Buffered() (int, error) {
	_, tx, err := d.status()
	return int(tx), err
}

// ModemControlLineState returns the asserted modem control input lines.
func (d *d2xxTerm) ModemControlLineState() (ModemControlLineState, error) {
	var status uint32
	if st := d.api.getModemStatus(d.handle, &status); st != ftOK {
		return 0, d2xxError(d.path, "FT_GetModemStatus", int(st))
	}
	var state ModemControlLineState
	if status&ftModemCTS != 0 {
		state |= modemControlLineState(ModemCTS)
	}
	if status&ftModemDCD != 0 {
		state |= modemControlLineState(ModemDCD)
	}
	if status&ftModemDSR != 0 {
		state |= modemControlLineState(ModemDSR)
	}
	if status&ftModemRI != 0 {
		state |= modemControlLineState(ModemRI)
	}
	return state, nil
}

// WaitModemControlLineChange blocks until a modem control input may have
// changed. The library notifies on any modem-status event or on received data,
// so the caller must read the state to tell what happened.
func (d *d2xxTerm) WaitModemControlLineChange(line ModemControlLine) error {
	if line < ModemCTS || line > ModemRI {
		return fmt.Errorf("invalid modem control line: %d", line)
	}
	if err := d.event.wait(); err != nil {
		return &os.PathError{Op: "wait for D2XX event", Path: d.path, Err: err}
	}
	return nil
}

// CancelModemControlLineWait unblocks a pending wait, to make shutdown prompt.
func (d *d2xxTerm) CancelModemControlLineWait() {
	d.event.signal()
}

func (d *d2xxTerm) Flush() error {
	if st := d.api.purge(d.handle, ftPurgeRX|ftPurgeTX); st != ftOK {
		return d2xxError(d.path, "FT_Purge", int(st))
	}
	return nil
}

// Drain waits for pending output to be transmitted. The library has no drain
// call, so poll the transmit queue.
func (d *d2xxTerm) Drain() error {
	for {
		_, tx, err := d.status()
		if err != nil || tx == 0 {
			return err
		}
		time.Sleep(time.Millisecond)
	}
}

func (d *d2xxTerm) status() (uint32, uint32, error) {
	var rx, tx, event uint32
	if st := d.api.getStatus(d.handle, &rx, &tx, &event); st != ftOK {
		return 0, 0, d2xxError(d.path, "FT_GetStatus", int(st))
	}
	return rx, tx, nil
}

// Restore does nothing: the library holds no saved line settings to put back.
func (d *d2xxTerm) Restore() error {
	return nil
}

func (d *d2xxTerm) Close() error {
	st := d.api.close(d.handle)
	d.handle = nil
	d.event.close()
	if st != ftOK {
		return d2xxError(d.path, "FT_Close", int(st))
	}
	return nil
}

var ftStatusNames = []string{
	"FT_OK",
	"FT_INVALID_HANDLE",
	"FT_DEVICE_NOT_FOUND",
	"FT_DEVICE_NOT_OPENED",
	"FT_IO_ERROR",
	"FT_INSUFFICIENT_RESOURCES",
	"FT_INVALID_PARAMETER",
	"FT_INVALID_BAUD_RATE",
	"FT_DEVICE_NOT_OPENED_FOR_ERASE",
	"FT_DEVICE_NOT_OPENED_FOR_WRITE",
	"FT_FAILED_TO_WRITE_DEVICE",
	"FT_EEPROM_READ_FAILED",
	"FT_EEPROM_WRITE_FAILED",
	"FT_EEPROM_ERASE_FAILED",
	"FT_EEPROM_NOT_PRESENT",
	"FT_EEPROM_NOT_PROGRAMMED",
	"FT_INVALID_ARGS",
	"FT_NOT_SUPPORTED",
	"FT_OTHER_ERROR",
	"FT_DEVICE_LIST_NOT_READY",
}

func d2xxError(path, op string, status int) error {
	var err error
	if status >= ftOK && status < len(ftStatusNames) {
		err = fmt.Errorf("D2XX: %s", ftStatusNames[status])
	} else {
		err = fmt.Errorf("D2XX: FT_STATUS %d", status)
	}
	return &os.PathError{Op: op, Path: path, Err: err}
}
