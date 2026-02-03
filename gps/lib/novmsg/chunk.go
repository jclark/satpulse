package novmsg

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/jclark/satpulse/gps/lib/fieldenc"
)

// Chunked interface for messages with variable-length structure
// that need to be read/written in chunks
type Chunked interface {
	// Chunks returns a push iterator that yields each chunk for sequential reading/writing
	Chunks() func(yield func(chunk any) bool)
}

// DecodeAsciiChunked decodes ASCII data fields into a message, handling both chunked and regular messages
func DecodeAsciiChunked(dataFields []string, msg any, messageName string) error {
	if chunkedMsg, ok := msg.(Chunked); ok {
		// Use the chunks iterator to parse the message
		fieldIndex := 0
		var err error
		for chunk := range chunkedMsg.Chunks() {
			fieldsConsumed, chunkErr := fieldenc.PartialDecode(dataFields[fieldIndex:], chunk)
			if chunkErr != nil {
				err = chunkErr
				break
			}
			fieldIndex += fieldsConsumed
		}
		if err != nil {
			return fmt.Errorf("parsing %s data: %v", messageName, err)
		}
		// Ensure all fields were consumed
		if fieldIndex != len(dataFields) {
			return fmt.Errorf("parsing %s data: expected to consume %d fields, consumed %d", messageName, len(dataFields), fieldIndex)
		}
	} else {
		err := fieldenc.Decode(dataFields, msg)
		if err != nil {
			return fmt.Errorf("parsing %s data: %v", messageName, err)
		}
	}
	return nil
}

// EncodeAsciiChunked encodes a message into ASCII data fields, handling both chunked and regular messages
func EncodeAsciiChunked(msg any, messageName string) ([]string, error) {
	var dataFields []string
	if chunkedMsg, ok := msg.(Chunked); ok {
		// Use the chunks iterator to serialize the message
		var err error
		for chunk := range chunkedMsg.Chunks() {
			chunkFields, chunkErr := fieldenc.Encode(chunk)
			if chunkErr != nil {
				err = chunkErr
				break
			}
			dataFields = append(dataFields, chunkFields...)
		}
		if err != nil {
			return nil, fmt.Errorf("encoding %s data: %v", messageName, err)
		}
	} else {
		// Use single encode for fixed-length messages
		var err error
		dataFields, err = fieldenc.Encode(msg)
		if err != nil {
			return nil, fmt.Errorf("encoding %s data: %v", messageName, err)
		}
	}
	return dataFields, nil
}

// ReadBinChunked reads binary data into a message, handling both chunked and regular messages
func ReadBinChunked(r io.Reader, msg any, messageName string) error {
	if chunkedMsg, ok := msg.(Chunked); ok {
		// Use the chunks iterator to read the message
		var err error
		for chunk := range chunkedMsg.Chunks() {
			if err = binary.Read(r, binary.LittleEndian, chunk); err != nil {
				break
			}
		}
		if err != nil {
			return fmt.Errorf("parsing %s: %v", messageName, err)
		}
	} else {
		// Use single read for fixed-length messages
		err := binary.Read(r, binary.LittleEndian, msg)
		if err != nil {
			return fmt.Errorf("parsing %s: %v", messageName, err)
		}
	}
	return nil
}

// WriteBinChunked writes a message to binary format, handling both chunked and regular messages
func WriteBinChunked(w io.Writer, msg any, messageName string) error {
	if chunkedMsg, ok := msg.(Chunked); ok {
		// Use the chunks iterator to write the message
		var err error
		for chunk := range chunkedMsg.Chunks() {
			if err = binary.Write(w, binary.LittleEndian, chunk); err != nil {
				break
			}
		}
		if err != nil {
			return fmt.Errorf("serializing %s: %v", messageName, err)
		}
	} else {
		// Use single write for fixed-length messages
		err := binary.Write(w, binary.LittleEndian, msg)
		if err != nil {
			return fmt.Errorf("serializing %s: %v", messageName, err)
		}
	}
	return nil
}
