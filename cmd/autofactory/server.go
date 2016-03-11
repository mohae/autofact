package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"time"

	"github.com/google/flatbuffers/go"
	"github.com/gorilla/websocket"
	influx "github.com/influxdata/influxdb/client/v2"
	"github.com/mohae/autofact"
	"github.com/mohae/autofact/client"
	"github.com/mohae/autofact/db"
	"github.com/mohae/autofact/message"
	"github.com/mohae/autofact/sysinfo"
)

// server is the container for a server's information and everything that it
// is tracking/serving.
type server struct {
	// ID of the server
	ID uint32
	// URL of the server
	url.URL
	// Period between pings
	PingPeriod time.Duration
	// How long to wait for a pong response before timing out
	PongWait time.Duration
	// Flatbuffers serialized default client config
	ClientCfg []byte
	// A map of clients, by ID
	Inventory inventory
	// TODO: add handling to prevent the same client from connecting
	// more than once:  this requires detection of reconnect of an
	// existing client vs an existing client maintaining multiple
	// con-current connections
	DB db.Bolt
	// InfluxDB client
	*InfluxClient
}

func newServer(id uint32) server {
	return server{
		ID:        id,
		Inventory: newInventory(),
	}
}

// LoadInventory populates the server's inventory from the database.  This
// is a cached list of clients.
func (s *server) LoadInventory() (int, error) {
	var n int
	clients, err := s.DB.Clients()
	if err != nil {
		return n, err
	}
	for i, c := range clients {
		s.Inventory.AddNodeInf(c.ID(), c)
		n = i
	}
	return n, nil
}

// connects to InfluxDB
func (s *server) connectToInfluxDB() error {
	var err error
	s.InfluxClient, err = newInfluxClient(influxDBName, influxAddress, influxUser, influxPassword)
	return err
}

// Node checks the inventory to see if the client exists.  If it exists,
// a Node is created from the client.Inf in the inventory.
func (s *server) Node(id uint32) (*Node, bool) {
	inf, ok := s.Inventory.ClientInf(id)
	if !ok {
		return nil, false
	}
	return &Node{
		Inf:          inf,
		Cfg:          client.GetRootAsCfg(clientCfg, 0),
		InfluxClient: s.InfluxClient,
	}, true
}

// NewNode creates a new Node, adds it to the server's inventory and
// returns the client.Inf to the caller.   If the save of the Node's inf to
// the database results in an error, it will be returned.
func (s *server) NewNode() (*Node, error) {
	// get a new client
	n := s.Inventory.NewNode()
	n.InfluxClient = s.InfluxClient
	// save the client info to the db
	err := s.DB.SaveClientInf(n.Inf)
	return n, err
}

// Node holds information about a client node.
type Node struct {
	*client.Inf
	*client.Cfg
	WS *websocket.Conn
	*InfluxClient
	isConnected bool
}

func newNode(id uint32) *Node {
	bldr := flatbuffers.NewBuilder(0)
	client.InfStart(bldr)
	client.InfAddID(bldr, id)
	bldr.Finish(client.InfEnd(bldr))
	return &Node{
		Inf: client.GetRootAsInf(bldr.Bytes[bldr.Head():], 0),
		Cfg: client.GetRootAsCfg(clientCfg, 0),
	}
}

// PingHandler is the handler for Pings.
func (n *Node) PingHandler(msg string) error {
	fmt.Printf("ping: %s\n", msg)
	return n.WS.WriteMessage(websocket.PongMessage, []byte("ping"))
}

// PongHandler is the handler for pongs.
func (n *Node) PongHandler(msg string) error {
	fmt.Printf("pong: %s\n", msg)
	return n.WS.WriteMessage(websocket.PingMessage, []byte("pong"))
}

// Listen listens for messages and handles them accordingly.  Binary messages
// are expected to be  Flatbuffer serialized bytes containing a Message.
func (n *Node) Listen(doneCh chan struct{}) {
	// loop until there's a done signal
	defer close(doneCh)
	for {
		typ, p, err := n.WS.ReadMessage()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading message: %s\n", err)
			if _, ok := err.(*websocket.CloseError); !ok {
				return
			}
			fmt.Println("client closed connection...waiting for reconnect")
			return
		}
		switch typ {
		case websocket.TextMessage:
			fmt.Printf("textmessage: %s\n", p)
			if bytes.Equal(p, autofact.AckMsg) {
				// if this is an acknowledgement message, do nothing
				continue
			}
			err := n.WS.WriteMessage(websocket.TextMessage, autofact.AckMsg)
			if err != nil {
				if _, ok := err.(*websocket.CloseError); !ok {
					return
				}
				fmt.Println("client closed connection...waiting for reconnect")
				return
			}
		case websocket.BinaryMessage:
			err = n.WS.WriteMessage(websocket.TextMessage, autofact.AckMsg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error writing binary message: %s\n", err)
				if _, ok := err.(*websocket.CloseError); !ok {
					return
				}
				fmt.Println("client closed connection...waiting for reconnect")
				return
			}
			n.processBinaryMessage(p)
		case websocket.CloseMessage:
			fmt.Printf("closemessage: %x\n", p)
			fmt.Println("client closed connection...waiting for reconnect")
			return
		}
	}
}

// WriteBinaryMessage serializes a message and writes it to the socket as
// a binary message.
func (n *Node) WriteBinaryMessage(k message.Kind, p []byte) {
	n.WS.WriteMessage(websocket.BinaryMessage, message.Serialize(n.Inf.ID(), k, p))
}

// binary messages are expected to be flatbuffer encoding of message.Message.
func (n *Node) processBinaryMessage(p []byte) error {
	// unmarshal the message
	msg := message.GetRootAsMessage(p, 0)
	// process according to kind
	k := message.Kind(msg.Kind())
	switch k {
	case message.CPUData:
		fmt.Println("cpu")
		cpu := sysinfo.GetRootAsCPUData(msg.DataBytes(), 0)
		tags := map[string]string{"host": string(n.Inf.Hostname()), "region": string(n.Inf.Region()), "cpu": string(cpu.CPUID())}
		fields := map[string]interface{}{
			"user":   float32(cpu.Usr()) / 100.0,
			"sys":    float32(cpu.Sys()) / 100.0,
			"iowait": float32(cpu.IOWait()) / 100.0,
			"idle":   float32(cpu.Idle()) / 100.0,
		}
		pt, err := influx.NewPoint("cpu", tags, fields, time.Unix(0, cpu.Timestamp()).UTC())
		n.InfluxClient.seriesCh <- Series{Data: []*influx.Point{pt}, err: err}
		return nil
	case message.MemData:
		fmt.Println("mem")
		mem := sysinfo.GetRootAsMemData(msg.DataBytes(), 0)
		tags := map[string]string{"host": string(n.Inf.Hostname()), "region": string(n.Inf.Region())}
		fields := map[string]interface{}{
			"mem-total":   mem.MemTotal(),
			"mem-used":    mem.MemUsed(),
			"mem-free":    mem.MemFree(),
			"mem-shared":  mem.MemShared(),
			"mem-buffers": mem.MemBuffers(),
			"cache-used":  mem.CacheUsed(),
			"cache-free":  mem.CacheFree(),
			"swap-total":  mem.SwapTotal(),
			"swap-used":   mem.SwapUsed(),
			"swap-free":   mem.SwapFree(),
		}
		pt, err := influx.NewPoint("memory", tags, fields, time.Unix(0, mem.Timestamp()).UTC())
		n.InfluxClient.seriesCh <- Series{Data: []*influx.Point{pt}, err: err}
		return nil
	default:
		fmt.Println("unknown message kind")
		fmt.Println(string(p))
	}
	return nil
}

// ClientCfg defines the client behavior, outside of connections.  This
// is serverside only and for loading the default clientCfg from a file.
// All communication of cfg data between Server and Client (Node) is done
// with Flatbuffers serialized client.Cfg.
type ClientCfg struct {
	RawHealthbeatInterval   string        `json:"healthbeat_interval"`
	HealthbeatInterval      time.Duration `json:"-"`
	RawHealthbeatPushPeriod string        `json:"healthbeat_push_period"`
	HealthbeatPushPeriod    time.Duration `json:"-"`
	RawPingPeriod           string        `json:"ping_period"`
	PingPeriod              time.Duration `json:"-"`
	RawPongWait             string        `json:"pong_wait"`
	PongWait                time.Duration `json:"-"`
	RawSaveInterval         string        `json:"save_interval"`
	SaveInterval            time.Duration `json:"-"`
	WriteWait               time.Duration `json:"-"`
}

// LoadClientCfg loads the client configuration from the specified file.
func (c *ClientCfg) Load(cfgFile string) error {
	b, err := ioutil.ReadFile(cfgFile)
	err = json.Unmarshal(b, c)
	if err != nil {
		return fmt.Errorf("error unmarshaling client cfg file %s: %s", cfgFile, err)
	}
	c.HealthbeatInterval, err = time.ParseDuration(c.RawHealthbeatInterval)
	if err != nil {
		return fmt.Errorf("error parsing healthbeat interval: %s", err)
	}
	c.HealthbeatPushPeriod, err = time.ParseDuration(c.RawHealthbeatPushPeriod)
	if err != nil {
		return fmt.Errorf("error parsing healthbeat push period %s", err)
	}
	c.SaveInterval, err = time.ParseDuration(c.RawSaveInterval)
	if err != nil {
		return fmt.Errorf("error parsing save interval %s", err)
	}
	c.PingPeriod, err = time.ParseDuration(c.RawPingPeriod)
	if err != nil {
		return fmt.Errorf("error parsing ping period %s", err)
	}
	c.PongWait, err = time.ParseDuration(c.RawPongWait)
	if err != nil {
		return fmt.Errorf("error parsing pong wait %s", err)
	}
	return nil
}

// Serialize serializes the struct.  The flatbuffers definition for this
// struct is in clientconf.fbs and the resulting definition is in
// client/ClientConf.go
func (c *ClientCfg) Serialize() []byte {
	bldr := flatbuffers.NewBuilder(0)
	client.CfgStart(bldr)
	client.CfgAddHealthbeatInterval(bldr, int64(c.HealthbeatInterval))
	client.CfgAddHealthbeatPushPeriod(bldr, int64(c.HealthbeatPushPeriod))
	client.CfgAddPingPeriod(bldr, int64(c.PingPeriod))
	client.CfgAddPongWait(bldr, int64(c.PongWait))
	client.CfgAddSaveInterval(bldr, int64(c.SaveInterval))
	bldr.Finish(client.CfgEnd(bldr))
	return bldr.Bytes[bldr.Head():]
}

// Deserialize deserializes the bytes into the struct.  The flatbuffers
// definition for this struct is in clientconf.fbs and the resulting
// definition is in client/ClientConf.go
func (c *ClientCfg) Deserialize(p []byte) {
	conf := client.GetRootAsCfg(p, 0)
	c.HealthbeatInterval = time.Duration(conf.HealthbeatInterval())
	c.HealthbeatPushPeriod = time.Duration(conf.HealthbeatPushPeriod())
	c.PingPeriod = time.Duration(conf.PingPeriod())
	c.PongWait = time.Duration(conf.PongWait())
	c.SaveInterval = time.Duration(conf.SaveInterval())
}
