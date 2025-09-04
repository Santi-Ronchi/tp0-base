package common

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// Protocol constants
const (
	FieldSeparator   = "|"
	MessageSeparator = "\n"
	BatchSeparator   = ";"
	MaxPacketSize    = 8192                       // 8KB máximo por paquete
	HeaderSize       = 4                          // 4 bytes para el length prefix
	MaxPayloadSize   = MaxPacketSize - HeaderSize // 8188 bytes para datos
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

// EstimateApuestaSize estimates the size in bytes of a serialized bet.
func EstimateApuestaSize(a Apuesta) int {
	serialized := SerializeApuesta(a)
	return len(serialized)
}

// SerializeBatch converts a batch of bets to wire format
// Format: bet1;bet2;bet3...
// ASSUMES: The batch is already sized to fit within the 8KB limit
func SerializeBatch(apuestas []Apuesta) []byte {
	if len(apuestas) == 0 {
		return []byte{}
	}

	var buffer bytes.Buffer

	for i, apuesta := range apuestas {
		if i > 0 {
			buffer.WriteString(BatchSeparator)
		}
		// Format of each bet in the batch
		msg := fmt.Sprintf("%d%s%s%s%s%s%s%s%s%s%d",
			apuesta.Agency,
			FieldSeparator,
			apuesta.FirstName,
			FieldSeparator,
			apuesta.LastName,
			FieldSeparator,
			apuesta.Document,
			FieldSeparator,
			apuesta.Birthdate,
			FieldSeparator,
			apuesta.Number)
		buffer.WriteString(msg)
	}

	return buffer.Bytes()
}

// DeserializeApuesta converts wire format to Apuesta struct
func DeserializeApuesta(data []byte) (*Apuesta, error) {
	str := string(data)
	parts := strings.Split(str, FieldSeparator)

	if len(parts) != 6 {
		return nil, fmt.Errorf("invalid message format: expected 6 fields, got %d", len(parts))
	}

	agency, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid agency: %v", err)
	}

	number, err := strconv.Atoi(parts[5])
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

// DeserializeBatch converts the wire format to a slice of Bets
func DeserializeBatch(data []byte) ([]*Apuesta, error) {
	str := string(data)

	// If empty, return empty slice
	if len(str) == 0 {
		return []*Apuesta{}, nil
	}

	// Split by batch separator
	betsStr := strings.Split(str, BatchSeparator)
	apuestas := make([]*Apuesta, 0, len(betsStr))

	for _, betStr := range betsStr {
		if betStr == "" {
			continue
		}
		apuesta, err := DeserializeApuesta([]byte(betStr))
		if err != nil {
			return nil, fmt.Errorf("error deserializing bet in batch: %v", err)
		}
		apuestas = append(apuestas, apuesta)
	}

	return apuestas, nil
}

// CreateMessage creates a length-prefixed message
// Format: [4 bytes length][message data]
// ASSUMES: The data has already been validated to fit within the limit.
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
