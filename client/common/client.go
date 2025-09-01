package common

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"strconv"
	"strings"
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
	Agency    int    `json:"agency"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Document  string `json:"document"`
	Birthdate string `json:"birthdate"`
	Number    int    `json:"number"`
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
		//log.Warningf("action: connect | result: retry | client_id: %v | attempt: %d | error: %v",
		//	c.config.ID, attempt, err)
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

// sendCompleteMessage sends a message ensuring no short-write occurs
func (c *Client) sendCompleteMessage(data []byte) error {
	totalSent := 0
	dataLen := len(data)

	for totalSent < dataLen {
		n, err := c.conn.Write(data[totalSent:])
		if err != nil {
			return err
		}
		totalSent += n
	}
	return nil
}

// readCompleteMessage reads until newline ensuring no short-read occurs
func (c *Client) readCompleteMessage() (string, error) {
	reader := bufio.NewReader(c.conn)
	msg, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(msg), nil
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

	// There is an autoincremental msgID to identify every message sent
	// Messages if the message amount threshold has not been surpassed
	for msgID := 1; msgID <= c.config.LoopAmount; msgID++ {
		select {
		case <-c.stop:
			return
		default:
		}

		// Serialize bet to JSON
		data, err := json.Marshal(apuesta)
		if err != nil {
			log.Errorf("action: serialize | result: fail | client_id: %v | error: %v", c.config.ID, err)
			continue
		}

		// Send complete message with newline
		message := append(data, '\n')
		if err := c.sendCompleteMessage(message); err != nil {
			log.Errorf("action: send_bet | result: fail | client_id: %v | error: %v", c.config.ID, err)
			return // si falla enviar → abortar
		}

		// Read server response
		response, err := c.readCompleteMessage()
		if err != nil {
			log.Errorf("action: receive_confirmation | result: fail | client_id: %v | error: %v", c.config.ID, err)
			return
		}

		// Check if server confirmed with OK
		if response == "OK" {
			// Log success as required by exercise
			log.Infof("action: apuesta_enviada | result: success | dni: %v | numero: %v", document, number)
		} else {
			log.Errorf("action: apuesta_enviada | result: fail | dni: %v | numero: %v | response: %v", document, number, response)
		}

		// Wait the configured period before sending a new bet
		time.Sleep(c.config.LoopPeriod)
	}
	log.Infof("action: loop_finished | result: success | client_id: %v", c.config.ID)
}
