# Terminal Package Cross-Platform Port Plan

## Functionality Requiring Platform-Specific Solutions

### Baud Rate Setting
**Purpose:** Configure serial port communication speed

**Problem:**
- Linux supports high-speed rates (460800, 921600) with constants `unix.B460800`, `unix.B921600`
- Darwin/FreeBSD lack these constants, maximum is B230400
- Linux can set speed via `Cflag` with `CBAUD`/`CBAUDEX` mask OR via `Ospeed`/`Ispeed`
- Darwin/FreeBSD only support `Ospeed`/`Ispeed` method, no `CBAUD`/`CBAUDEX` constants
- `Cflag` is `uint32` on Linux but `uint64` on Darwin/FreeBSD

**Solution:**
- Platform-specific baud rate tables (Linux includes high speeds, BSD doesn't)
- Platform-specific speed setting functions
- Linux: Try Cflag method first, fall back to Ospeed/Ispeed
- BSD: Only use Ospeed/Ispeed

### Terminal Attributes Get/Set
**Purpose:** Read and modify terminal settings

**Problem:**
- Different ioctl constants:
  - Linux: `TCGETS`, `TCSETS`, `TCSETSW`
  - Darwin/FreeBSD: `TIOCGETA`, `TIOCSETA`, `TIOCSETAW`

**Solution:**
- Platform-specific constants for termios operations
- Wrapper functions that use the appropriate constants

### Flush Input/Output
**Purpose:** Discard pending input/output data

**Problem:**
- Linux: `unix.IoctlSetInt(fd, unix.TCFLSH, unix.TCIOFLUSH)`
- Darwin/FreeBSD: `unix.IoctlSetInt(fd, unix.TIOCFLUSH, unix.FREAD|unix.FWRITE)`

**Solution:**
- Platform-specific flush implementation with different constants

### Serial Error Counting
**Purpose:** Track frame errors, parity errors, overruns, etc.

**Problem:**
- Linux has `TIOCGICOUNT` ioctl returning `serial_icounter_struct`
- Darwin/FreeBSD have no equivalent functionality

**Solution:**
- Make ErrorCounts include an `Available bool` field
- Linux: Implement full functionality, set Available=true
- Darwin/FreeBSD: Return zero counts with Available=false

### Device Type Detection
**Purpose:** Determine if device is UART, USB-serial, Bluetooth, etc.

**Problem:**
- Linux uses major/minor device numbers (e.g., major 188/189 = USB-serial)
- Darwin/FreeBSD have completely different device numbering schemes

**Solution:**
- Linux: Keep current major/minor number approach
- Darwin: Use device path patterns (`/dev/cu.usbserial*`, `/dev/cu.Bluetooth*`)
- FreeBSD: Use device path patterns (`/dev/cuaU*`, `/dev/cuau*`)

## Proposed File Structure

```
term/
├── term.go           # All cross-platform code
│                     # - Term struct and most methods
│                     # - Open, Read, Write, Close
│                     # - RawMode, Local, NoFlowControl, ReadTimeout
│                     # - All code that uses only common unix constants
│
├── term_linux.go     # Linux-specific implementations
│                     # - baudRates table with high speeds
│                     # - speed(), setSpeed() using CBAUD
│                     # - getAttr(), setAttrNow(), setAttrDrain() using TCGETS/TCSETS
│                     # - Flush() using TCFLSH
│                     # - GetErrorCounts() with full implementation
│                     # - DevKind() using major/minor numbers
│                     # - IoctlGetSerialICounter()
│
├── term_bsd.go       # Shared BSD code (build tag: //go:build darwin || freebsd)
│                     # - baudRates table without high speeds
│                     # - speed(), setSpeed() using Ospeed/Ispeed only
│                     # - getAttr(), setAttrNow(), setAttrDrain() using TIOCGETA/TIOCSETA
│                     # - Flush() using TIOCFLUSH
│                     # - GetErrorCounts() returning Available=false
│
├── term_darwin.go    # Darwin-specific overrides (if needed)
│                     # - DevKind() using path patterns
│
├── term_freebsd.go   # FreeBSD-specific overrides (if needed)
│                     # - DevKind() using path patterns
│
├── ioctl.go          # Linux-only (build tag: //go:build linux)
├── types.go          # Linux header definitions (build tag: //go:build ignore)
└── ztypes.go         # Generated types
```

### Implementation Strategy

1. Move these functions from term.go to platform files:
   - `speed()`, `setSpeed()` (due to CBAUD differences)
   - `getAttr()`, `setAttrNow()`, `setAttrDrain()` (different ioctls)
   - `Flush()` (different ioctls)
   - `GetErrorCounts()` (Linux-only feature)
   - `DevKind()` (platform-specific detection)
   - `baudRates` table (different speeds supported)

2. Keep in term.go:
   - All other methods that work identically across platforms
   - Methods that only use common constants like CLOCAL, CRTSCTS, etc.

3. Start with term_bsd.go for shared Darwin/FreeBSD code
   - Only create term_darwin.go and term_freebsd.go if we find differences (likely only for DevKind)