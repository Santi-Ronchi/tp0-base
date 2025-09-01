package common

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

// Protocol constants
const (
	FieldSeparator   = "|"
	MessageSeparator = "\n"
)

// SerializeApuesta converts Apuesta to wire format using custom protocol
// Format: agency|firstName|lastName|document|birthdate|number
func SerializeApuesta(a Apuesta) []byte {
	msg := fmt.Sprintf("%d%s%s%s%s%s%s%s%s%s%d",
		a.Agency,
		FieldSeparator,
		a.FirstName,
		FieldSeparator,
		a.LastName,
		FieldSeparator,
		a.Document,
		FieldSeparator,
		a.Birthdate,
		FieldSeparator,
		a.Number)
	return []byte(msg)
}

// DeserializeApuesta converts wire format to Apuesta struct
func DeserializeApuesta(data []byte) (*Apuesta, error) {
	str := string(data)
	parts := strings.Split(str, FieldSeparator)

	if len(parts) != 6 {
		return nil, fmt.Errorf("invalid message format: expected 6 fields, got %d", len(parts))
	}

	var agency, number int
	_, err := fmt.Sscanf(parts[0], "%d", &agency)
	if err != nil {
		return nil, fmt.Errorf("invalid agency: %v", err)
	}

	_, err = fmt.Sscanf(parts[5], "%d", &number)
	if err != nil {
		return nil, fmt.Errorf("invalid number: %v", err)
	}

	return &Apuesta{
		Agency:    agency,
		FirstName: parts[1],
		LastName:  parts[2],
		Document:  parts[3],
		Birthdate: parts[4],
		Number:    number,
	}, nil
}

// CreateMessage creates a length-prefixed message
// Format: [4 bytes length][message data]
func CreateMessage(data []byte) ([]byte, error) {
	var buf bytes.Buffer

	// Write message length as 4-byte big-endian integer
	msgLen := uint32(len(data))
	if err := binary.Write(&buf, binary.BigEndian, msgLen); err != nil {
		return nil, err
	}

	// Write the actual message
	buf.Write(data)

	return buf.Bytes(), nil
}

// ReadMessageLength reads the 4-byte length prefix from a byte slice
func ReadMessageLength(data []byte) (uint32, error) {
	if len(data) < 4 {
		return 0, fmt.Errorf("insufficient data for length prefix")
	}

	var msgLen uint32
	if err := binary.Read(bytes.NewReader(data[:4]), binary.BigEndian, &msgLen); err != nil {
		return 0, err
	}

	return msgLen, nil
}
