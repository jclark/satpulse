//go:build !linux

package phc

func IfDriverFlags(ifname string) (DriverFlags, error) {
	return 0, errPHCNotSupported
}

func IfDriverNameFlags(driverName string) (DriverFlags, bool) {
	panic(errPHCNotSupported)
}

func IfDriverName(ifname string) (name string, err error) {
	return "", errPHCNotSupported
}

func IfPhyID(ifname string) (PhyID, error) {
	return 0, errPHCNotSupported
}

func PhyIDDriverFlags(id PhyID) DriverFlags {
	panic(errPHCNotSupported)
}