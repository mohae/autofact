package client

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/google/flatbuffers/go"
)

// ConnCfg holds the connection information for a node.  This is all that is
// persisted on a client node.
type ConnCfg struct {
	ServerAddress      string        `json:"server_address"`
	ServerPort         string        `json:"server_port"`
	ServerID           uint32        `json:"server_id"`
	RawConnectInterval string        `json:"connect_interval"`
	ConnectInterval    time.Duration `json:"-"`
	RawConnectPeriod   string        `json:"connect_period"`
	ConnectPeriod      time.Duration `json:"-"`
	filename           string
}

// LoadConnCfg loads the client config file.  Errors are logged but not
// returned.  TODO: revisit this design decision.
func (c *ConnCfg) Load(cfgFile string) error {
	c.filename = cfgFile
	b, err := ioutil.ReadFile(cfgFile)
	if err != nil {
		return fmt.Errorf("read of config file failed: %s", err)
	}
	err = json.Unmarshal(b, &c)
	if err != nil {
		return fmt.Errorf("error unmarshaling confg file %s: %s", cfgFile, err)
	}
	c.ConnectInterval, err = time.ParseDuration(c.RawConnectInterval)
	if err != nil {
		return fmt.Errorf("error parsing connect interval: %s", err)
	}
	c.ConnectPeriod, err = time.ParseDuration(c.RawConnectPeriod)
	if err != nil {
		return fmt.Errorf("error parsing connect period: %s", err)
	}
	return nil
}

func (c *ConnCfg) Save() error {
	j, err := json.MarshalIndent(c, "", "\t")
	if err != nil {
		return fmt.Errorf("fail: cfg save: %s", err)
	}
	f, err := os.OpenFile(c.filename, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0640)
	if err != nil {
		return fmt.Errorf("fail: cfg save: %s", err)
	}
	defer f.Close()
	n, err := f.Write(j)
	if err != nil {
		return fmt.Errorf("fail: cfg save: %s", err)
	}
	if n != len(j) {
		return fmt.Errorf("fail: cfg save: short write: wrote %d of %d bytes", n, len(j))
	}
	return nil
}

// Serialize serializes the struct.  The flatbuffers definition for this
// struct is in clientconf.fbs and the resulting definition is in
// client/ClientConf.go
func (c *Cfg) Serialize() []byte {
	bldr := flatbuffers.NewBuilder(0)
	CfgStart(bldr)
	CfgAddHealthbeatInterval(bldr, c.HealthbeatInterval())
	CfgAddHealthbeatPushPeriod(bldr, c.HealthbeatPushPeriod())
	CfgAddPingPeriod(bldr, c.PingPeriod())
	CfgAddPongWait(bldr, c.PongWait())
	CfgAddSaveInterval(bldr, c.SaveInterval())
	bldr.Finish(CfgEnd(bldr))
	return bldr.Bytes[bldr.Head():]
}

// Deserialize deserializes the bytes into the current Cfg.
func (c *Cfg) Deserialize(p []byte) {
	c = GetRootAsCfg(p, 0)
}