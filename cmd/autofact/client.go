package main

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mohae/autofact"
	"github.com/mohae/autofact/conf"
	"github.com/mohae/autofact/message"
	cpuutil "github.com/mohae/joefriday/cpu/utilization/flat"
	netf "github.com/mohae/joefriday/net/usage/flat"
	load "github.com/mohae/joefriday/sysinfo/load"
	loadf "github.com/mohae/joefriday/sysinfo/load/flat"
	memf "github.com/mohae/joefriday/sysinfo/mem/flat"
	"github.com/mohae/snoflinga"
	czap "github.com/mohae/zap"
	"github.com/uber-go/zap"
)

const IDLen = 8

// Client is anything that talks to the server.
type Client struct {
	// The Autofact Path
	AutoPath string
	// Conn holds the configuration for connecting to the server.
	conf.Conn
	// Conf holds the client configuration (how the client behaves).
	Conf *conf.Client
	// this lock is for everything except messages or other things that are
	// already threadsafe.
	mu sync.Mutex
	// The websocket connection that this client uses.
	WS *websocket.Conn
	// Channel for outbound binary messages.  The message is assumed to be a
	// websocket.Binary type
	sendB       chan []byte
	sendStr     chan string
	isConnected bool
	ServerURL   url.URL
	genLock     sync.Mutex
	idGen       snoflinga.Generator
	// The func is assigned on creation as the implementation could change
	// depending on where the output is directed and/or requested format.
	LoadAvg func() ([]byte, error)
}

func NewClient(c conf.Conn) *Client {
	return &Client{
		Conn: c,
		// A really small buffer:
		// TODO: rethink this vis-a-vis what happens when recipient isn't there
		// or if it goes away during sending and possibly caching items to be sent.
		sendB:   make(chan []byte, 8),
		sendStr: make(chan string, 8),
	}
}

// Connect handles connecting to the server and returns the connection status.
// The client will attempt to connect until it has either succeeded or the
// connection retry period has been exceeded.  A retry is done every 5 seconds.
//
// If the client is already connected, nothing will be done.
func (c *Client) Connect() bool {
	// If already connected, return that fact.
	if c.IsConnected() {
		return true
	}
	start := time.Now()
	retryEnd := start.Add(c.ConnectPeriod.Duration)
	// connect to server; retry until the retry period has expired
	for {
		if time.Now().After(retryEnd) {
			log.Warn(
				"timed out",
				zap.String("op", "connect"),
				zap.String("server", c.ServerURL.String()),
			)
			return false
		}
		err := c.DialServer()
		if err == nil {
			break
		}
		time.Sleep(c.ConnectInterval.Duration)
		log.Debug(
			"failed: retrying...",
			zap.String("op", "connect"),
			zap.String("server", c.ServerURL.String()),
		)
	}
	// Send the ID
	err := c.WS.WriteMessage(websocket.TextMessage, c.Conn.ID)
	if err != nil {
		log.Error(
			err.Error(),
			zap.String("op", "send id"),
			zap.String("id", string(c.Conn.ID)),
		)
		c.WS.Close()
		return false
	}

	// read messages until we get an EOT
handshake:
	for {
		typ, p, err := c.WS.ReadMessage()
		if err != nil {
			log.Error(
				err.Error(),
				zap.String("op", "read message"),
			)
			c.WS.Close()
			return false
		}
		switch typ {
		case websocket.BinaryMessage:
			// process according to message kind
			msg := message.GetRootAsMessage(p, 0)
			switch message.Kind(msg.Kind()) {
			case message.ClientConf:
				c.Conf = conf.GetRootAsClient(msg.DataBytes(), 0)
				// If there's a new ID, persist it/
				if bytes.Compare(c.Conn.ID, c.Conf.IDBytes()) != 0 {
					c.Conn.ID = c.Conf.IDBytes() // save the ID; if it was an
					err := c.Save()
					if err != nil {
						log.Error(
							err.Error(),
							zap.String("op", "save config"),
							zap.String("file", c.Filename),
						)
						c.WS.Close()
						return false
					}
				}
			case message.EOT:
				break handshake
			default:
				log.Error("unknown message type received during handshake")
				return false
			}
		case websocket.TextMessage:
			fmt.Printf("%s\n", string(p))
		default:
			log.Error(
				"unknown message type",
				zap.Int("type", typ),
				zap.Base64("message", p),
			)
			c.WS.Close()
			return false
		}
	}
	log.Debug(
		"success",
		zap.String("op", "connect"),
		zap.String("id", c.ServerURL.String()),
	)
	c.mu.Lock()
	c.isConnected = true
	c.mu.Unlock()
	// assume that the ID is now set: get a snowflake Generator
	c.genLock.Lock()
	c.idGen = snoflinga.New(c.Conn.ID)
	c.genLock.Unlock()
	return true
}

func (c *Client) DialServer() error {
	var err error
	c.WS, _, err = websocket.DefaultDialer.Dial(c.ServerURL.String(), nil)
	return err
}

// NewMessage creates a new message of type Kind using the received bytes.
// The MessageID is a snowflake using the client's ID and the current time.
func (c *Client) NewMessage(k message.Kind, p []byte) []byte {
	c.genLock.Lock()
	defer c.genLock.Unlock()
	return message.Serialize(c.idGen.Snowflake(), k, p)
}

func (c *Client) MessageWriter(doneCh chan struct{}) {
	defer close(doneCh)
	for {
		select {
		case p, ok := <-c.sendB:
			// don't send if not connected
			if !c.IsConnected() {
				// TODO add write to db for persistence instead.
				continue
			}
			if !ok {
				c.WS.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			err := c.WS.WriteMessage(websocket.BinaryMessage, p)
			if err != nil {
				log.Error(
					err.Error(),
					zap.String("op", "write message"),
				)
			}
			// TODO does this need to handle healthbeat?
		}
	}
}

func (c *Client) Reconnect() bool {
	c.mu.Lock()
	c.isConnected = false
	c.mu.Unlock()
	for i := 0; i < 4; i++ {
		b := c.Connect()
		if b {
			log.Debug(
				"reconnected",
				zap.String("op", "reconnect"),
				zap.String("server", c.ServerURL.String()),
			)
			return b
		}
	}
	return false
}

func (c *Client) Listen(doneCh chan struct{}) {
	// loop until there's a done signal
	defer close(doneCh)
	for {
		typ, p, err := c.WS.ReadMessage()
		if err != nil {
			log.Error(
				err.Error(),
				zap.String("op", "read message"),
			)
			if _, ok := err.(*websocket.CloseError); !ok {
				return
			}
			log.Debug(
				"connection closed: reconnecting",
				zap.String("op", "read message"),
			)
			connected := c.Reconnect()
			if connected {
				log.Debug(
					"connection re-established",
					zap.String("op", "read message"),
				)
				continue
			}
			log.Error(
				"reconnect failed",
				zap.String("op", "read message"),
			)
			return
		}
		switch typ {
		case websocket.TextMessage:
			log.Debug(
				string(p),
				zap.String("op", "read message"),
				zap.String("type", "text"),
			)
			if bytes.Equal(p, autofact.LoadAvg) {
				p, err = c.LoadAvg()
				if err != nil {
					log.Error(
						err.Error(),
						zap.String("op", "healthbeat"),
						zap.String("data", "loadavg"),
					)
					continue
				}
				err = c.WS.WriteMessage(websocket.BinaryMessage, c.NewMessage(message.LoadAvg, p))
				if err != nil {
					if _, ok := err.(*websocket.CloseError); !ok {
						log.Error(
							err.Error(),
							zap.String("op", "write message"),
							zap.String("type", "loadavg"),
						)
						return
					}
					log.Debug(
						"connection closed: reconnecting",
						zap.String("op", "write message"),
						zap.String("type", "loadavg"),
					)
					connected := c.Reconnect()
					if connected {
						log.Debug(
							"connection re-established",
							zap.String("op", "write message"),
							zap.String("type", "loadavg"),
						)
						continue
					}
					log.Error(
						"reconnect failed",
						zap.String("op", "write message"),
						zap.String("type", "loadavg"),
					)
					return
				}
				c.WS.WriteMessage(websocket.TextMessage, c.NewMessage(message.LoadAvg, p))
				continue
			}
		case websocket.BinaryMessage:
			// nothing right now
			// TODO validate that nothing is thecorrect thing here
		case websocket.CloseMessage:
			log.Debug(
				"connection closed by remote: reconnecting",
			)
			connected := c.Reconnect()
			if connected {
				log.Debug(
					"remote connection re-established",
				)
				continue
			}
			log.Error(
				"remote reconnect failed",
			)
			return
		}
	}
}

// IsConnected returns if the client is connected.
func (c *Client) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.isConnected
}

func (c *Client) CPUUtilization(doneCh chan struct{}) {
	// An interval of 0 means don't collect meminfo
	if c.Conf.CPUUtilizationPeriod() == 0 {
		return
	}
	// ticker for cpu utilization data
	cpuTicker, err := cpuutil.NewTicker(time.Duration(c.Conf.CPUUtilizationPeriod()))
	if err != nil {
		log.Error(
			err.Error(),
			zap.String("op", "create ticker"),
			zap.String("type", "cpuutilization"),
		)
		return
	}
	cpuTickr := cpuTicker.(*cpuutil.Ticker)
	// make sure the resources get cleaned up
	defer cpuTickr.Close()
	defer cpuTickr.Stop()
	for {
		select {
		case data, ok := <-cpuTickr.Data:
			if !ok {
				log.Error(
					"ticker closed",
					zap.String("type", "cpuutilization"),
				)
				return
			}
			c.sendB <- c.NewMessage(message.CPUUtilization, data)
		case <-doneCh:
			return
		}
	}
}

func (c *Client) MemInfo(doneCh chan struct{}) {
	// An interval of 0 means don't collect meminfo
	if c.Conf.MemInfoPeriod() == 0 {
		return
	}
	// ticker for meminfo data
	memTicker, err := memf.NewTicker(time.Duration(c.Conf.MemInfoPeriod()))
	if err != nil {
		log.Error(
			err.Error(),
			zap.String("op", "create ticker"),
			zap.String("type", "meminfo"),
		)
		return
	}
	memTickr := memTicker.(*memf.Ticker)
	defer memTickr.Close()
	defer memTickr.Stop()
	for {
		select {
		case data, ok := <-memTickr.Data:
			if !ok {
				log.Error(
					"ticker closed",
					zap.String("type", "meminfo"),
				)
				return
			}
			c.sendB <- c.NewMessage(message.MemInfo, data)
		case <-doneCh:
			return
		}
	}
}

func (c *Client) NetUsage(doneCh chan struct{}) {
	// An interval of 0 means don't collect meminfo
	if c.Conf.NetUsagePeriod() == 0 {
		return
	}
	// ticker for network usage data
	netTicker, err := netf.NewTicker(time.Duration(c.Conf.NetUsagePeriod()))
	if err != nil {
		log.Error(
			err.Error(),
			zap.String("op", "create ticker"),
			zap.String("type", "netusage"),
		)
		return
	}
	netTickr := netTicker.(*netf.Ticker)
	// make sure the resources get cleaned up
	defer netTickr.Close()
	defer netTickr.Stop()
	for {
		select {
		case data, ok := <-netTickr.Data:
			if !ok {
				log.Error(
					"ticker closed",
					zap.String("type", "netusage"),
				)
				return
			}
			c.sendB <- c.NewMessage(message.NetUsage, data)
		case <-doneCh:
			return
		}
	}
}

// binary messages are expected to be flatbuffer encoding of message.Message.
func (c *Client) processBinaryMessage(p []byte) error {
	// unmarshal the message
	msg := message.GetRootAsMessage(p, 0)
	// process according to kind
	k := message.Kind(msg.Kind())
	switch k {
	case message.ClientConf:
		c.Conf.Deserialize(msg.DataBytes())
	default:
		log.Warn(
			"unknown message kind",
			zap.String("type", k.String()),
			zap.Base64("data", p),
		)
	}
	return nil
}

// HealthbeatLocal gets the healthbeat on a ticker and saves it to the datalog.
// This is only used when serverless.  The client's configured HealthbeatPeriod
// is used for the ticker; for non-serverless environments that value is
// ignored as the healthbeat is gathered on a server pull request.
func (c *Client) HealthbeatLocal(done chan struct{}) {
	// If this was set to 0; don't do a healthbeat.
	if c.Conf.HealthbeatPeriod() == 0 {
		return
	}
	loadOut := dataLog.With(
		czap.String("id", string(c.Conf.IDBytes())),
	)
	ticker := time.NewTicker(time.Duration(c.Conf.HealthbeatPeriod()))
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			// request the Healthbeat; serveClient will handle the response.
			l, err := LoadAvg()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: healthbeat error: %s", string(c.Conf.IDBytes()), err)
				return
			} // log the data
			loadOut.Warn(
				"loadavg",
				czap.Float64("one", l.One),
				czap.Float64("five", l.Five),
				czap.Float64("fifteen", l.Fifteen),
			)
		case <-done:
			return
		}
	}
}

// LoadAvgFB gets the current loadavg as Flatbuffer serialized bytes.
func LoadAvgFB() ([]byte, error) {
	return loadf.Get()
}

// LoadAvg gets the current loadavg.
func LoadAvg() (load.LoadAvg, error) {
	return load.Get()
}
