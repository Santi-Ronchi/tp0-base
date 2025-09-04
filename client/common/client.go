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

// StreamingBatchProcessor manages the processing of streaming CSV files
type StreamingBatchProcessor struct {
	client       *Client
	currentBatch []Apuesta
	currentSize  int
	agency       int
	batchesSent  int
}

// NewStreamingBatchProcessor creates a new streaming batch processor
func (c *Client) NewStreamingBatchProcessor() *StreamingBatchProcessor {
	// Converts Client ID to int to be usable as agency
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

// AddApuesta adds bet to batch, sending it if the limit is reached
func (sbp *StreamingBatchProcessor) AddApuesta(apuesta Apuesta) error {
	// If there's a batchSize limit set, use that as the limit
	if sbp.client.config.BatchSize > 0 && len(sbp.currentBatch) >= sbp.client.config.BatchSize {
		if err := sbp.sendCurrentBatch(); err != nil {
			return err
		}
	} else {
		// Fallback to size limit if no BatchSize set
		apuestaSize := EstimateApuestaSize(apuesta)
		separatorSize := 0
		if len(sbp.currentBatch) > 0 {
			separatorSize = len(BatchSeparator)
		}

		// If adding this bet sends the size over the limit, send instead
		if sbp.currentSize+apuestaSize+separatorSize > MaxPayloadSize {
			if len(sbp.currentBatch) > 0 {
				if err := sbp.sendCurrentBatch(); err != nil {
					return err
				}
			}
		}
	}

	// Add bet to batch
	sbp.currentBatch = append(sbp.currentBatch, apuesta)
	if sbp.client.config.BatchSize <= 0 {
		// Only calculate size if we use a fixed BatchSize
		apuestaSize := EstimateApuestaSize(apuesta)
		sbp.currentSize += apuestaSize
		if len(sbp.currentBatch) > 1 {
			sbp.currentSize += len(BatchSeparator)
		}
	}

	return nil
}

// sendCurrentBatch sends current batch and resets
func (sbp *StreamingBatchProcessor) sendCurrentBatch() error {
	if len(sbp.currentBatch) == 0 {
		return nil
	}

	// Verif batch limit
	if sbp.client.config.LoopAmount > 0 && sbp.batchesSent >= sbp.client.config.LoopAmount {
		return fmt.Errorf("batch limit reached")
	}

	// Serialize batch
	data := SerializeBatch(sbp.currentBatch)

	log.Debugf("Sending batch: %d bets, packet size: %d bytes (max: %d)",
		len(sbp.currentBatch), len(data)+HeaderSize, MaxPacketSize)

	// Send batch
	if err := sbp.client.sendMessage(data); err != nil {
		return fmt.Errorf("error sending batch: %v", err)
	}

	
	response, err := sbp.client.readMessage()
	if err != nil {
		return fmt.Errorf("error receiving confirmation: %v", err)
	}

	// Verify answer is OK
	if response == "OK" {
		sbp.batchesSent++
		log.Infof("action: batch_enviado | result: success | client_id: %v | batch_size: %d",
			sbp.client.config.ID, len(sbp.currentBatch))

		// If delay is set, use it between batches.
		if sbp.client.config.LoopPeriod > 0 {
			time.Sleep(sbp.client.config.LoopPeriod)
		}
	} else {
		return fmt.Errorf("server responded with: %v", response)
	}

	// reset currrent batch
	sbp.currentBatch = sbp.currentBatch[:0]
	sbp.currentSize = 0

	return nil
}

// FlushBatch sends remaining batch
func (sbp *StreamingBatchProcessor) FlushBatch() error {
	return sbp.sendCurrentBatch()
}

// processCSVStreaming processes the CSV line per line to avoid loading all to memory
func (c *Client) processCSVStreaming(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("error opening CSV file: %v", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	processor := c.NewStreamingBatchProcessor()

	lineNumber := 0

	// Read first line
	firstLine, err := reader.Read()
	if err != nil {
		if err == io.EOF {
			log.Warningf("CSV file is empty")
			return nil
		}
		return fmt.Errorf("error reading CSV: %v", err)
	}
	lineNumber++

	// Verify if header
	isHeader := false
	if len(firstLine) == 5 {
		if firstLine[0] == "nombre" || firstLine[0] == "Nombre" || firstLine[0] == "NOMBRE" {
			isHeader = true
		}
	}

	// if not header, process first line
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

	// Process the rest of the file line per line
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

		// Process record
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

	// Send remaining batch
	if err := processor.FlushBatch(); err != nil {
		if err.Error() != "batch limit reached" {
			return fmt.Errorf("error flushing final batch: %v", err)
		}
	}

	log.Infof("action: loop_finished | result: success | client_id: %v | batches_sent: %d",
		c.config.ID, processor.batchesSent)

	return nil
}

// parseCSVRecord converts a CSV record to an Apuesta structure
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

	// Process CSV on streaming
	csvFilename := fmt.Sprintf("/data/agency-%s.csv", c.config.ID)

	if err := c.processCSVStreaming(csvFilename); err != nil {
		log.Errorf("action: process_csv_streaming | result: fail | client_id: %v | error: %v", c.config.ID, err)
		return
	}
}
