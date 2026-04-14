package uncmsg

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jclark/satpulse/gps/lib/latin1z"
)

// Message IDs for version-related messages
const (
	VersionID MsgID = 37
)

// ProductModel represents the product model enum from Version message
type ProductModel uint32

const (
	ProductModelUnknown ProductModel = 0
	ProductModelUM982   ProductModel = 17
	ProductModelUM980   ProductModel = 18
	ProductModelUM960   ProductModel = 19
	ProductModelUM960L  ProductModel = 24
	ProductModelUM981   ProductModel = 26
	ProductModelUM981S  ProductModel = 31
	ProductModelUMD982  ProductModel = 40
	ProductModelUMD981  ProductModel = 41
	ProductModelUMD981S ProductModel = 42
	ProductModelUM981C  ProductModel = 43
	ProductModelUB9A0   ProductModel = 52
	ProductModelUBD9A0  ProductModel = 53
	ProductModelUMD960  ProductModel = 62
	ProductModelUMD980  ProductModel = 63
	ProductModelUM980C  ProductModel = 64
	ProductModelUM982C  ProductModel = 65
)

// String returns the ASCII representation of ProductModel
func (pm ProductModel) String() string {
	switch pm {
	case ProductModelUnknown:
		return "UNKNOWN"
	case ProductModelUM982:
		return "UM982"
	case ProductModelUM980:
		return "UM980"
	case ProductModelUM960:
		return "UM960"
	case ProductModelUM960L:
		return "UM960L"
	case ProductModelUM981:
		return "UM981"
	case ProductModelUM981S:
		return "UM981S"
	case ProductModelUMD982:
		return "UMD982"
	case ProductModelUMD981:
		return "UMD981"
	case ProductModelUMD981S:
		return "UMD981S"
	case ProductModelUM981C:
		return "UM981C"
	case ProductModelUB9A0:
		return "UB9A0"
	case ProductModelUBD9A0:
		return "UBD9A0"
	case ProductModelUMD960:
		return "UMD960"
	case ProductModelUMD980:
		return "UMD980"
	case ProductModelUM980C:
		return "UM980C"
	case ProductModelUM982C:
		return "UM982C"
	default:
		return fmt.Sprintf("%d", uint32(pm))
	}
}

// ParseProductModel converts an ASCII product model string to ProductModel enum
func ParseProductModel(s string) (ProductModel, error) {
	switch s {
	case "UNKNOWN":
		return ProductModelUnknown, nil
	case "UM982":
		return ProductModelUM982, nil
	case "UM980":
		return ProductModelUM980, nil
	case "UM960":
		return ProductModelUM960, nil
	case "UM960L":
		return ProductModelUM960L, nil
	case "UM981":
		return ProductModelUM981, nil
	case "UM981S":
		return ProductModelUM981S, nil
	case "UMD982":
		return ProductModelUMD982, nil
	case "UMD981":
		return ProductModelUMD981, nil
	case "UMD981S":
		return ProductModelUMD981S, nil
	case "UM981C":
		return ProductModelUM981C, nil
	case "UB9A0":
		return ProductModelUB9A0, nil
	case "UBD9A0":
		return ProductModelUBD9A0, nil
	case "UMD960":
		return ProductModelUMD960, nil
	case "UMD980":
		return ProductModelUMD980, nil
	case "UM980C":
		return ProductModelUM980C, nil
	case "UM982C":
		return ProductModelUM982C, nil
	default:
		// Try to parse as number
		if n, err := strconv.ParseUint(s, 10, 32); err == nil {
			return ProductModel(n), nil
		}
		return 0, fmt.Errorf("unknown product model: %s", s)
	}
}

// UnmarshalText implements encoding.TextUnmarshaler for fieldenc support
func (pm *ProductModel) UnmarshalText(text []byte) error {
	val, err := ParseProductModel(string(text))
	if err != nil {
		return err
	}
	*pm = val
	return nil
}

// MarshalText implements encoding.TextMarshaler for fieldenc support
func (pm ProductModel) MarshalText() ([]byte, error) {
	return []byte(pm.String()), nil
}

// Version represents the VERSION message from a Unicore receiver.
// Message ID: 37
// Contains product model, software version, serial number, and authorization information.
type Version struct {
	Type        ProductModel       // Product model
	SwVersion   latin1z.StringZ33  // Firmware version string
	Auth        latin1z.StringZ129 // Authorization type string
	Psn         latin1z.StringZ66  // Part number and Serial number
	EfuseID     latin1z.StringZ33  // Board ID
	CompileTime latin1z.StringZ43  // Firmware compile time
}

// ID returns the message ID for VERSION
func (v *Version) ID() (MsgID, string) {
	return VersionID, "VERSIONA"
}

// quotedAscii marks Version as using quoted ASCII fields
func (v *Version) quotedAscii() {}

// QuotedFieldCount returns the expected number of fields for Version message
func (v *Version) QuotedFieldCount() int {
	return 6 // Type, SwVersion, Auth, Psn, EfuseID, CompileTime
}

// BuildNumber extracts the build number from SwVersion field.
// Handles both binary format ("13504") and ASCII format ("R4.10Build13504").
// Returns the build number as an integer.
func (v *Version) BuildNumber() (int, error) {
	swVer := v.SwVersion.String()

	// Try parsing as pure number first (binary format)
	if buildNum, err := strconv.Atoi(swVer); err == nil {
		return buildNum, nil
	}

	// Look for "Build" followed by digits (ASCII format)
	buildIndex := strings.Index(swVer, "Build")
	if buildIndex == -1 {
		return 0, fmt.Errorf("no build number found in version: %s", swVer)
	}

	buildStr := swVer[buildIndex+5:] // Skip "Build"
	buildNum, err := strconv.Atoi(buildStr)
	if err != nil {
		return 0, fmt.Errorf("invalid build number in version %s: %v", swVer, err)
	}

	return buildNum, nil
}

func init() {
	// Register known message types
	regMsg[Version]("VERSION")
}
