package pmc

// This file deals with the MANAGEMENT_ERROR_STATUS TLV.
// Defined in 15.5.4 of the standard.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

type MgmtErrorStatus struct {
	MgmtErrorID MgmtErrorID
	MgmtID      MgmtID
	_           [4]byte
	DisplayData *string
}

type MgmtErrorStatusMsg = MgmtMsgWithValue[MgmtErrorStatus]

type MgmtErrorID uint16

const (
	_ MgmtErrorID = iota
	MEIDResponseTooBig
	MEIDNoSuchID
	MEIDWrongLength
	MEIDWrongValue
	MEIDNotSetable
	MEIDNotSupported
	MEIDUnpopulated
	MEIDGeneralError MgmtErrorID = 0xFFFE
)

func (meid MgmtErrorID) String() string {
	// return the same strings as in the standard
	switch meid {
	case MEIDResponseTooBig:
		return "RESPONSE_TOO_BIG"
	case MEIDNoSuchID:
		return "NO_SUCH_ID"
	case MEIDWrongLength:
		return "WRONG_LENGTH"
	case MEIDWrongValue:
		return "WRONG_VALUE"
	case MEIDNotSetable:
		return "NOT_SETABLE"
	case MEIDNotSupported:
		return "NOT_SUPPORTED"
	case MEIDUnpopulated:
		return "UNPOPULATED"
	case MEIDGeneralError:
		return "GENERAL_ERROR"
	}
	return fmt.Sprintf("0x%0x", uint16(meid))
}

func (m *MgmtErrorStatus) WriteBinaryTo(w io.Writer) error {
	if err := binary.Write(w, binary.BigEndian, &m.MgmtErrorID); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, &m.MgmtID); err != nil {
		return err
	}
	var reserved [4]byte
	if err := binary.Write(w, binary.BigEndian, &reserved); err != nil {
		return err
	}
	if m.DisplayData == nil {
		return nil
	}
	text := *m.DisplayData
	if err := writePTPText(w, text); err != nil {
		return err
	}
	if len(text)%2 == 0 {
		var padding uint8
		if err := binary.Write(w, binary.BigEndian, &padding); err != nil {
			return err
		}
	}
	return nil
}

func unmarshalMgmtErrorStatus(r *bytes.Reader, m *MgmtErrorStatus) error {
	if err := binary.Read(r, binary.BigEndian, &m.MgmtErrorID); err != nil {
		return fmt.Errorf("unmarshaling MgmtErrorID failed: %w", err)
	}
	if err := binary.Read(r, binary.BigEndian, &m.MgmtID); err != nil {
		return fmt.Errorf("unmarshaling MgmtID failed: %w", err)
	}
	var reserved [4]byte
	if err := binary.Read(r, binary.BigEndian, &reserved); err != nil {
		return fmt.Errorf("unmarshaling MgmtErrorStatus reserved bytes failed: %w", err)
	}
	// The DisplayData can be omitted completely.
	if r.Len() == 0 {
		return nil
	}
	text, err := readPTPText(r)
	if err != nil {
		return err
	}
	m.DisplayData = &text
	// The padding needs to make the length of the TLV even.
	// Since there is a single byte of length, even lengths require a padding byte.
	if len(text)%2 == 0 {
		var padding uint8
		if err := binary.Read(r, binary.BigEndian, &padding); err != nil {
			return fmt.Errorf("unmarshaling MgmtErrorStatus padding failed: %w", err)
		}
	}
	return nil
}

func writePTPText(w io.Writer, s string) error {
	l := len(s)
	if l > 255 {
		return fmt.Errorf("PTPText too long: %d", len(s))
	}
	lengthField := uint8(l)
	if err := binary.Write(w, binary.BigEndian, &lengthField); err != nil {
		return err
	}
	textField := ([]byte)(s)
	return binary.Write(w, binary.BigEndian, &textField)
}

func readPTPText(r io.Reader) (string, error) {
	lengthField := uint8(0)
	if err := binary.Read(r, binary.BigEndian, &lengthField); err != nil {
		return "", fmt.Errorf("unmarshaling PTPText length failed: %w", err)
	}
	textField := make([]byte, lengthField)
	if err := binary.Read(r, binary.BigEndian, &textField); err != nil {
		return "", fmt.Errorf("unmarshaling PTPText string failed: %w", err)
	}
	return string(textField), nil
}
