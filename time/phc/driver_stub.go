//go:build !linux

package phc

func IfDriverFlags(ifname string) (DriverFlags, error) {
	return 0, ErrNotSupported
}

func IfDriverNameFlags(driverName string) (DriverFlags, bool) {
	panic(ErrNotSupported)
}

func IfDriverName(ifname string) (name string, err error) {
	return "", ErrNotSupported
}

func IfPhyID(ifname string) (PhyID, error) {
	return 0, ErrNotSupported
}

func PhyIDDriverFlags(id PhyID) DriverFlags {
	panic(ErrNotSupported)
}