package common

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
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

// EstimateApuestaSize estima el tamaño en bytes de una apuesta serializada
func EstimateApuestaSize(a Apuesta) int {
	serialized := SerializeApuesta(a)
	return len(serialized)
}

// SerializeBatch convierte un batch de apuestas al formato wire
// Format: apuesta1;apuesta2;apuesta3...
// Retorna error si el batch excede el tamaño máximo
func SerializeBatch(apuestas []Apuesta) []byte {
	var buffer bytes.Buffer

	for i, apuesta := range apuestas {
		if i > 0 {
			buffer.WriteString(BatchSeparator)
		}
		// Formato de cada apuesta en el batch
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

// SerializeBatchSafe serializa un batch verificando que no exceda el tamaño máximo
// Si excede, retorna un error
func SerializeBatchSafe(apuestas []Apuesta) ([]byte, error) {
	data := SerializeBatch(apuestas)

	if len(data) > MaxPayloadSize {
		return nil, fmt.Errorf("batch size (%d bytes) exceeds maximum payload size (%d bytes)",
			len(data), MaxPayloadSize)
	}

	return data, nil
}

// SplitBatchToFitSize divide un batch grande en múltiples batches que caben en el límite
func SplitBatchToFitSize(apuestas []Apuesta) [][]Apuesta {
	var batches [][]Apuesta
	var currentBatch []Apuesta
	currentSize := 0

	for _, apuesta := range apuestas {
		apuestaSize := EstimateApuestaSize(apuesta)

		// Si agregar esta apuesta excedería el límite, crear un nuevo batch
		// También consideramos el separador (1 byte) si no es la primera apuesta
		separatorSize := 0
		if len(currentBatch) > 0 {
			separatorSize = len(BatchSeparator)
		}

		if currentSize+apuestaSize+separatorSize > MaxPayloadSize {
			// Si el batch actual tiene apuestas, guardarlo
			if len(currentBatch) > 0 {
				batches = append(batches, currentBatch)
				currentBatch = []Apuesta{}
				currentSize = 0
			}

			// Si una sola apuesta excede el límite, hay un problema
			if apuestaSize > MaxPayloadSize {
				// Log warning pero incluir la apuesta de todos modos
				// En producción, podrías querer manejar esto diferente
				fmt.Printf("WARNING: Single bet exceeds max payload size: %d bytes\n", apuestaSize)
			}
		}

		currentBatch = append(currentBatch, apuesta)
		currentSize += apuestaSize
		if len(currentBatch) > 1 {
			currentSize += separatorSize
		}
	}

	// Agregar el último batch si tiene apuestas
	if len(currentBatch) > 0 {
		batches = append(batches, currentBatch)
	}

	return batches
}

// CalculateOptimalBatchSize calcula el tamaño óptimo de batch basado en el tamaño promedio de apuesta
func CalculateOptimalBatchSize(sampleApuestas []Apuesta) int {
	if len(sampleApuestas) == 0 {
		// Valor por defecto conservador
		return 50
	}

	// Calcular tamaño promedio
	totalSize := 0
	for _, apuesta := range sampleApuestas {
		totalSize += EstimateApuestaSize(apuesta)
	}
	avgSize := totalSize / len(sampleApuestas)

	// Agregar overhead por separadores (estimado)
	avgSizeWithSeparator := avgSize + len(BatchSeparator)

	// Calcular cuántas apuestas caben en el límite con un margen de seguridad (90%)
	safeLimit := int(math.Floor(float64(MaxPayloadSize) * 0.9))
	optimalBatchSize := safeLimit / avgSizeWithSeparator

	return optimalBatchSize
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

// DeserializeBatch convierte el formato wire a un slice de Apuestas
func DeserializeBatch(data []byte) ([]*Apuesta, error) {
	str := string(data)

	// Si está vacío, retornar slice vacío
	if len(str) == 0 {
		return []*Apuesta{}, nil
	}

	// Dividir por el separador de batch
	betsStr := strings.Split(str, BatchSeparator)
	apuestas := make([]*Apuesta, 0, len(betsStr))

	for _, betStr := range betsStr {
		apuesta, err := DeserializeApuesta([]byte(betStr))
		if err != nil {
			return nil, fmt.Errorf("error deserializing bet in batch: %v", err)
		}
		apuestas = append(apuestas, apuesta)
	}

	return apuestas, nil
}

// CreateMessage creates a length-prefixed message with size validation
// Format: [4 bytes length][message data]
func CreateMessage(data []byte) ([]byte, error) {
	// Validar que el mensaje no exceda el tamaño máximo
	if len(data) > MaxPayloadSize {
		return nil, fmt.Errorf("message size (%d bytes) exceeds maximum allowed (%d bytes)",
			len(data), MaxPayloadSize)
	}

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

	// Validar que la longitud no exceda el máximo permitido
	if msgLen > MaxPayloadSize {
		return 0, fmt.Errorf("message length (%d) exceeds maximum allowed (%d)",
			msgLen, MaxPayloadSize)
	}

	return msgLen, nil
}

// ValidatePacketSize verifica que un paquete completo no exceda el límite
func ValidatePacketSize(data []byte) error {
	if len(data) > MaxPacketSize {
		return fmt.Errorf("packet size (%d bytes) exceeds maximum (%d bytes)",
			len(data), MaxPacketSize)
	}
	return nil
}
