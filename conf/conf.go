package conf

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/google/flatbuffers/go"
	"github.com/mohae/autofact/util"
)

// Conf is used to hold flag arguments passed on start
type Conf struct {
	args []*flag.Flag // all flags that were visited
}

// Visited builds a list of the names of the flags that were passed as args.
func (c *Conf) Visited(a *flag.Flag) {
	c.args = append(c.args, a)
}

// Args returns all of the args that were passed.
func (c *Conf) Args() []*flag.Flag {
	return c.args
}

// Flag returns the requested flag, if it was set, or nil.
func (c *Conf) Flag(s string) *flag.Flag {
	for _, v := range c.args {
		if s == v.Name {
			return v
		}
	}
	return nil
}

// Conn holds the connection information for a node.  This is all that is
// persisted on a client node.
type Conn struct {
	ID              []byte        `json:"id"`
	ServerAddress   string        `json:"server_address"`
	ServerPort      string        `json:"server_port"`
	ServerID        uint32        `json:"server_id"`
	ConnectInterval util.Duration `json:"connect_interval"`
	ConnectPeriod   util.Duration `json:"connect_period"`
	filename        string
	Conf
}

// LoadConn loads the config file.  The Conn's filename is set during this
// operation.
// TODO: revisit this design decision.
func (c *Conn) Load(name string) error {
	c.filename = name
	b, err := ioutil.ReadFile(name)
	if err != nil {
		return err
	}
	err = json.Unmarshal(b, &c)
	if err != nil {
		return fmt.Errorf("error unmarshaling confg file %s: %s", name, err)
	}
	return nil
}

func (c *Conn) Save() error {
	j, err := json.MarshalIndent(c, "", "\t")
	if err != nil {
		return fmt.Errorf("fail: marshal conn cfg to JSON: %s\n", err)
	}
	f, err := os.OpenFile(c.filename, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0640)
	if err != nil {
		return fmt.Errorf("fail: conn cfg save: %s\n", err)
	}
	defer f.Close()
	n, err := f.Write(j)
	if err != nil {
		return fmt.Errorf("fail: conn cfg save: %s\n", err)
	}
	if n != len(j) {
		return fmt.Errorf("fail: conn cfg save: short write: wrote %d of %d bytes\n", n, len(j))
	}
	return nil
}

func (c *Conn) SetFilename(v string) {
	c.filename = v
}

// Serialize serializes the Client conf.
func (c *Client) Serialize() []byte {
	bldr := flatbuffers.NewBuilder(0)
	id := bldr.CreateByteVector(c.IDBytes())
	ClientStart(bldr)
	ClientAddID(bldr, id)
	bldr.Finish(ClientEnd(bldr))
	return bldr.Bytes[bldr.Head():]
}

// Deserialize deserializes the bytes into the current Client Conf.
func (c *Client) Deserialize(p []byte) {
	c = GetRootAsClient(p, 0)
}
