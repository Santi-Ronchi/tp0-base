package common

import (
	"encoding/csv"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/op/go-logging"
)

var log = logging.MustGetLogger("log")

// ClientConfig Configuration used by the client
type ClientConfig struct {
	ID            string
	ServerAddress string
	LoopAmount    int
	LoopPeriod    time.Duration
	BatchSize     int // Agregado para configurar tamaño del batch
}

// Client Entity that encapsulates how
type Client struct {
	config ClientConfig
	conn   net.Conn
	stop   chan struct{}
}

type Apuesta struct {
	Agency    int
	FirstName string
	LastName  string
	Document  string
	Birthdate string
	Number    int
}

func (c *Client) Shutdown() {
	if c.conn != nil {
		c.conn.Close()
		log.Infof("action: close_socket | result: success | client_id: %v", c.config.ID)
	}
	close(c.stop)
}

// NewClient Initializes a new client receiving the configuration
// as a parameter
func NewClient(config ClientConfig) *Client {
	return &Client{
		config: config,
		stop:   make(chan struct{}),
	}
}

// CreateClientSocket Initializes client socket. In case of
// failure, error is printed in stdout/stderr and exit 1
// is returned
func (c *Client) createClientSocket(maxRetries int, delay time.Duration) error {
	var err error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err = c.attemptConnection()
		if err == nil {
			return nil
		}
		time.Sleep(delay)
	}
	return err
}

func (c *Client) attemptConnection() error {
	conn, err := net.Dial("tcp", c.config.ServerAddress)
	if err != nil {
		return err
	}
	c.conn = conn
	log.Infof("action: connect | result: success | client_id: %v | server_address: %v",
		c.config.ID, c.config.ServerAddress)
	return nil
}

// sendMessage sends a message with length prefix to handle message boundaries
func (c *Client) sendMessage(data []byte) error {
	// Create length-prefixed message using protocol module
	msg, err := CreateMessage(data)
	if err != nil {
		return err
	}

	// Send complete message
	totalSent := 0
	for totalSent < len(msg) {
		n, err := c.conn.Write(msg[totalSent:])
		if err != nil {
			return err
		}
		totalSent += n
	}
	return nil
}

// readMessage reads a message with length prefix
func (c *Client) readMessage() (string, error) {
	// Read 4-byte length prefix
	lengthBuf := make([]byte, 4)
	totalRead := 0
	for totalRead < 4 {
		n, err := c.conn.Read(lengthBuf[totalRead:])
		if err != nil {
			return "", err
		}
		totalRead += n
	}

	// Parse message length using protocol module
	msgLen, err := ReadMessageLength(lengthBuf)
	if err != nil {
		return "", err
	}

	// Read the message body
	msgBuf := make([]byte, msgLen)
	totalRead = 0
	for totalRead < int(msgLen) {
		n, err := c.conn.Read(msgBuf[totalRead:])
		if err != nil {
			return "", err
		}
		totalRead += n
	}

	return string(msgBuf), nil
}

// loadApuestasFromCSV lee las apuestas desde el archivo CSV
func (c *Client) loadApuestasFromCSV(filename string) ([]Apuesta, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("error opening CSV file: %v", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)

	// Skip header if exists - check if first line contains headers
	firstLine, err := reader.Read()
	if err != nil {
		if err == io.EOF {
			return []Apuesta{}, nil // Archivo vacío
		}
		return nil, fmt.Errorf("error reading CSV: %v", err)
	}

	// Check if it's a header line (contains "nombre" or similar)
	isHeader := false
	if len(firstLine) == 5 {
		// Simple check: if first field contains letters, it's probably a header
		if firstLine[0] == "nombre" || firstLine[0] == "Nombre" || firstLine[0] == "NOMBRE" {
			isHeader = true
		}
	}

	var apuestas []Apuesta

	// Convertir el ID del cliente a int para usarlo como agency
	var agency int
	if _, err := fmt.Sscanf(c.config.ID, "%d", &agency); err != nil {
		// Si el ID no es numérico, usar el valor 1 por defecto
		log.Warningf("Client ID is not numeric (%s), using agency=1", c.config.ID)
		agency = 1
	}

	// Si no era header, procesar la primera línea
	if !isHeader && len(firstLine) == 5 {
		var number int
		if _, err := fmt.Sscanf(firstLine[4], "%d", &number); err == nil {
			apuesta := Apuesta{
				Agency:    agency,
				FirstName: firstLine[0],
				LastName:  firstLine[1],
				Document:  firstLine[2],
				Birthdate: firstLine[3],
				Number:    number,
			}
			apuestas = append(apuestas, apuesta)
		}
	}

	// Procesar el resto del archivo
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Warningf("Error reading CSV record: %v", err)
			continue
		}

		// Esperamos formato CSV: nombre,apellido,documento,nacimiento,numero (5 campos)
		if len(record) != 5 {
			log.Warningf("Skipping invalid record (expected 5 fields, got %d): %v", len(record), record)
			continue
		}

		var number int
		if _, err := fmt.Sscanf(record[4], "%d", &number); err != nil {
			log.Warningf("Invalid number in record: %v", record)
			continue
		}

		apuesta := Apuesta{
			Agency:    agency, // ID del cliente es el ID de la agencia
			FirstName: record[0],
			LastName:  record[1],
			Document:  record[2],
			Birthdate: record[3],
			Number:    number,
		}
		apuestas = append(apuestas, apuesta)
	}

	return apuestas, nil
}

// StartClientLoop Send messages to the client until some time threshold is met
func (c *Client) StartClientLoop() {
	if err := c.createClientSocket(3, time.Second); err != nil {
		log.Errorf("action: connect | result: fail | client_id: %v | error: %v", c.config.ID, err)
		return
	}
	defer func() {
		if c.conn != nil {
			c.conn.Close()
			log.Infof("action: close_socket | result: success | client_id: %v", c.config.ID)
		}
	}()

	csvFilename := fmt.Sprintf("/data/agency-%s.csv", c.config.ID)

	apuestas, err := c.loadApuestasFromCSV(csvFilename)
	if err != nil {
		log.Errorf("action: load_csv | result: fail | client_id: %v | error: %v", c.config.ID, err)
		return
	}

	log.Infof("action: load_csv | result: success | client_id: %v | total_apuestas: %d", c.config.ID, len(apuestas))

	// Si no hay apuestas, salir
	if len(apuestas) == 0 {
		log.Warningf("No bets found in CSV file for client %v", c.config.ID)
		return
	}

	// Calcular número de batches necesarios
	totalBatches := (len(apuestas) + c.config.BatchSize - 1) / c.config.BatchSize

	// Limitar al número configurado de loops (cada loop es un batch)
	batchesToSend := totalBatches
	if c.config.LoopAmount > 0 && c.config.LoopAmount < batchesToSend {
		batchesToSend = c.config.LoopAmount
	}

	log.Debugf("Total apuestas: %d, Batch size: %d, Total batches: %d, Batches to send: %d",
		len(apuestas), c.config.BatchSize, totalBatches, batchesToSend)

	batchesSent := 0

	for i := 0; i < len(apuestas) && batchesSent < batchesToSend; i += c.config.BatchSize {
		select {
		case <-c.stop:
			return
		default:
		}

		// Determinar el rango del batch actual
		end := i + c.config.BatchSize
		if end > len(apuestas) {
			end = len(apuestas)
		}

		batch := apuestas[i:end]

		// Serializar el batch usando el protocolo
		data := SerializeBatch(batch)

		// Verificar que el tamaño no exceda 8KB
		if len(data) > 8192 {
			log.Warningf("action: batch_size_warning | batch_size: %d bytes | max: 8192", len(data))
		}

		// Enviar el batch
		if err := c.sendMessage(data); err != nil {
			log.Errorf("action: send_batch | result: fail | client_id: %v | error: %v", c.config.ID, err)
			return
		}

		// Leer respuesta del servidor
		response, err := c.readMessage()
		if err != nil {
			log.Errorf("action: receive_confirmation | result: fail | client_id: %v | error: %v", c.config.ID, err)
			return
		}

		// Verificar respuesta
		if response == "OK" {
			log.Infof("action: batch_enviado | result: success | client_id: %v | batch_size: %d", c.config.ID, len(batch))
			// Log adicional para debugging
			log.Debugf("Batch %d/%d sent successfully with %d bets", batchesSent+1, batchesToSend, len(batch))
		} else {
			log.Errorf("action: batch_enviado | result: fail | client_id: %v | batch_size: %d | response: %v",
				c.config.ID, len(batch), response)
			// En caso de error, salir del loop
			return
		}

		batchesSent++

		// Si todavía hay más batches para enviar
		if batchesSent < batchesToSend {
			time.Sleep(c.config.LoopPeriod)
		}
	}

	log.Infof("action: loop_finished | result: success | client_id: %v | batches_sent: %d", c.config.ID, batchesSent)
}
