package common

import (
	"net"
	"os"
	"strconv"
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

	agencyStr := os.Getenv("AGENCIA")
	numberStr := os.Getenv("NUMERO")
	document := os.Getenv("DOCUMENTO")

	agency, err := strconv.Atoi(agencyStr)
	if err != nil {
		log.Criticalf("action: config | result: fail | field: AGENCIA | error: %v", err)
		return
	}

	number, err := strconv.Atoi(numberStr)
	if err != nil {
		log.Criticalf("action: config | result: fail | field: NUMERO | error: %v", err)
		return
	}

	apuesta := Apuesta{
		Agency:    agency,
		FirstName: os.Getenv("NOMBRE"),
		LastName:  os.Getenv("APELLIDO"),
		Document:  document,
		Birthdate: os.Getenv("NACIMIENTO"),
		Number:    number,
	}

	// Messages loop
	for msgID := 1; msgID <= c.config.LoopAmount; msgID++ {
		select {
		case <-c.stop:
			return
		default:
		}

		// Serialize bet using custom protocol from protocol.go
		data := SerializeApuesta(apuesta)

		// Send message using length-prefixed protocol
		if err := c.sendMessage(data); err != nil {
			log.Errorf("action: send_bet | result: fail | client_id: %v | error: %v", c.config.ID, err)
			return
		}

		// Read server response
		response, err := c.readMessage()
		if err != nil {
			log.Errorf("action: receive_confirmation | result: fail | client_id: %v | error: %v", c.config.ID, err)
			return
		}

		// Check if server confirmed with OK
		if response == "OK" {
			log.Infof("action: apuesta_enviada | result: success | dni: %v | numero: %v", document, number)
		} else {
			log.Errorf("action: apuesta_enviada | result: fail | dni: %v | numero: %v | response: %v", document, number, response)
		}

		// Wait the configured period before sending a new bet
		time.Sleep(c.config.LoopPeriod)
	}
	log.Infof("action: loop_finished | result: success | client_id: %v", c.config.ID)
}
