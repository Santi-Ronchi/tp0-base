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

// StreamingBatchProcessor maneja el procesamiento de CSV en streaming
type StreamingBatchProcessor struct {
	client       *Client
	currentBatch []Apuesta
	currentSize  int
	agency       int
	batchesSent  int
}

// NewStreamingBatchProcessor crea un nuevo procesador de batches streaming
func (c *Client) NewStreamingBatchProcessor() *StreamingBatchProcessor {
	// Convertir el ID del cliente a int para usarlo como agency
	var agency int
	if _, err := fmt.Sscanf(c.config.ID, "%d", &agency); err != nil {
		log.Warningf("Client ID is not numeric (%s), using agency=1", c.config.ID)
		agency = 1
	}

	return &StreamingBatchProcessor{
		client:       c,
		currentBatch: make([]Apuesta, 0),
		currentSize:  0,
		agency:       agency,
		batchesSent:  0,
	}
}

// AddApuesta añade una apuesta al batch actual, enviándolo si se alcanza el límite
func (sbp *StreamingBatchProcessor) AddApuesta(apuesta Apuesta) error {
	// Si tenemos un BatchSize configurado, usarlo como límite primario
	if sbp.client.config.BatchSize > 0 && len(sbp.currentBatch) >= sbp.client.config.BatchSize {
		if err := sbp.sendCurrentBatch(); err != nil {
			return err
		}
	} else {
		// Fallback a límite de tamaño si no hay BatchSize configurado
		apuestaSize := EstimateApuestaSize(apuesta)
		separatorSize := 0
		if len(sbp.currentBatch) > 0 {
			separatorSize = len(BatchSeparator)
		}

		// Si agregar esta apuesta excedería el límite, enviar el batch actual
		if sbp.currentSize+apuestaSize+separatorSize > MaxPayloadSize {
			if len(sbp.currentBatch) > 0 {
				if err := sbp.sendCurrentBatch(); err != nil {
					return err
				}
			}
		}
	}

	// Agregar la apuesta al batch actual
	sbp.currentBatch = append(sbp.currentBatch, apuesta)
	if sbp.client.config.BatchSize <= 0 {
		// Solo calcular tamaño si no usamos BatchSize fijo
		apuestaSize := EstimateApuestaSize(apuesta)
		sbp.currentSize += apuestaSize
		if len(sbp.currentBatch) > 1 {
			sbp.currentSize += len(BatchSeparator)
		}
	}

	return nil
}

// sendCurrentBatch envía el batch actual y lo resetea
func (sbp *StreamingBatchProcessor) sendCurrentBatch() error {
	if len(sbp.currentBatch) == 0 {
		return nil
	}

	// Verificar si hemos alcanzado el límite de batches
	if sbp.client.config.LoopAmount > 0 && sbp.batchesSent >= sbp.client.config.LoopAmount {
		return fmt.Errorf("batch limit reached")
	}

	// Serializar el batch
	data := SerializeBatch(sbp.currentBatch)

	log.Debugf("Sending batch: %d bets, packet size: %d bytes (max: %d)",
		len(sbp.currentBatch), len(data)+HeaderSize, MaxPacketSize)

	// Enviar el batch
	if err := sbp.client.sendMessage(data); err != nil {
		return fmt.Errorf("error sending batch: %v", err)
	}

	// Leer respuesta del servidor
	response, err := sbp.client.readMessage()
	if err != nil {
		return fmt.Errorf("error receiving confirmation: %v", err)
	}

	// Verificar respuesta
	if response == "OK" {
		sbp.batchesSent++
		log.Infof("action: batch_enviado | result: success | client_id: %v | batch_size: %d",
			sbp.client.config.ID, len(sbp.currentBatch))

		// Aplicar delay entre batches si está configurado
		if sbp.client.config.LoopPeriod > 0 {
			time.Sleep(sbp.client.config.LoopPeriod)
		}
	} else {
		return fmt.Errorf("server responded with: %v", response)
	}

	// Resetear el batch actual
	sbp.currentBatch = sbp.currentBatch[:0] // Reutilizar el slice subyacente
	sbp.currentSize = 0

	return nil
}

// FlushBatch envía cualquier batch restante
func (sbp *StreamingBatchProcessor) FlushBatch() error {
	return sbp.sendCurrentBatch()
}

// processCSVStreaming procesa el CSV línea por línea sin cargar todo en memoria
func (c *Client) processCSVStreaming(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("error opening CSV file: %v", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	processor := c.NewStreamingBatchProcessor()

	lineNumber := 0

	// Leer primera línea para verificar si es header
	firstLine, err := reader.Read()
	if err != nil {
		if err == io.EOF {
			log.Warningf("CSV file is empty")
			return nil
		}
		return fmt.Errorf("error reading CSV: %v", err)
	}
	lineNumber++

	// Verificar si es header
	isHeader := false
	if len(firstLine) == 5 {
		if firstLine[0] == "nombre" || firstLine[0] == "Nombre" || firstLine[0] == "NOMBRE" {
			isHeader = true
		}
	}

	// Si no es header, procesar la primera línea
	if !isHeader && len(firstLine) == 5 {
		if apuesta, err := c.parseCSVRecord(firstLine, processor.agency); err == nil {
			if err := processor.AddApuesta(*apuesta); err != nil {
				if err.Error() == "batch limit reached" {
					log.Infof("Batch limit reached, stopping processing")
					return nil
				}
				return err
			}
		} else {
			log.Warningf("Error parsing first record (line %d): %v", lineNumber, err)
		}
	}

	// Procesar el resto del archivo línea por línea
	for {
		select {
		case <-c.stop:
			return fmt.Errorf("processing stopped by signal")
		default:
		}

		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Warningf("Error reading CSV record (line %d): %v", lineNumber+1, err)
			lineNumber++
			continue
		}
		lineNumber++

		// Procesar el record
		if apuesta, err := c.parseCSVRecord(record, processor.agency); err == nil {
			if err := processor.AddApuesta(*apuesta); err != nil {
				if err.Error() == "batch limit reached" {
					log.Infof("Batch limit reached, stopping processing")
					break
				}
				return fmt.Errorf("error processing record (line %d): %v", lineNumber, err)
			}
		} else {
			log.Warningf("Error parsing record (line %d): %v", lineNumber, err)
		}
	}

	// Enviar cualquier batch restante
	if err := processor.FlushBatch(); err != nil {
		// Si es porque ya alcanzamos el límite, no es un error
		if err.Error() != "batch limit reached" {
			return fmt.Errorf("error flushing final batch: %v", err)
		}
	}

	log.Infof("action: loop_finished | result: success | client_id: %v | batches_sent: %d",
		c.config.ID, processor.batchesSent)

	return nil
}

// parseCSVRecord convierte un record de CSV a una estructura Apuesta
func (c *Client) parseCSVRecord(record []string, agency int) (*Apuesta, error) {
	if len(record) != 5 {
		return nil, fmt.Errorf("invalid record format: expected 5 fields, got %d", len(record))
	}

	var number int
	if _, err := fmt.Sscanf(record[4], "%d", &number); err != nil {
		return nil, fmt.Errorf("invalid number: %v", err)
	}

	return &Apuesta{
		Agency:    agency,
		FirstName: record[0],
		LastName:  record[1],
		Document:  record[2],
		Birthdate: record[3],
		Number:    number,
	}, nil
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

	// Procesar CSV en streaming
	csvFilename := fmt.Sprintf("/data/agency-%s.csv", c.config.ID)

	if err := c.processCSVStreaming(csvFilename); err != nil {
		log.Errorf("action: process_csv_streaming | result: fail | client_id: %v | error: %v", c.config.ID, err)
		return
	}
}
