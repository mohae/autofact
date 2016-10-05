// automatically generated by the FlatBuffers compiler, do not modify

package conf

import (
	flatbuffers "github.com/google/flatbuffers/go"
)
type Client struct {
	_tab flatbuffers.Table
}

func GetRootAsClient(buf []byte, offset flatbuffers.UOffsetT) *Client {
	n := flatbuffers.GetUOffsetT(buf[offset:])
	x := &Client{}
	x.Init(buf, n + offset)
	return x
}

func (rcv *Client) Init(buf []byte, i flatbuffers.UOffsetT) {
	rcv._tab.Bytes = buf
	rcv._tab.Pos = i
}

func (rcv *Client) ID(j int) byte {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		a := rcv._tab.Vector(o)
		return rcv._tab.GetByte(a + flatbuffers.UOffsetT(j * 1))
	}
	return 0
}

func (rcv *Client) IDLength() int {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		return rcv._tab.VectorLen(o)
	}
	return 0
}

func (rcv *Client) IDBytes() []byte {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		return rcv._tab.ByteVector(o + rcv._tab.Pos)
	}
	return nil
}

func (rcv *Client) Hostname() []byte {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(6))
	if o != 0 {
		return rcv._tab.ByteVector(o + rcv._tab.Pos)
	}
	return nil
}

func (rcv *Client) Region() []byte {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(8))
	if o != 0 {
		return rcv._tab.ByteVector(o + rcv._tab.Pos)
	}
	return nil
}

func (rcv *Client) Zone() []byte {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(10))
	if o != 0 {
		return rcv._tab.ByteVector(o + rcv._tab.Pos)
	}
	return nil
}

func (rcv *Client) DataCenter() []byte {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(12))
	if o != 0 {
		return rcv._tab.ByteVector(o + rcv._tab.Pos)
	}
	return nil
}

func (rcv *Client) MemInfoPeriod() int64 {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(14))
	if o != 0 {
		return rcv._tab.GetInt64(o + rcv._tab.Pos)
	}
	return 0
}

func (rcv *Client) NetUsagePeriod() int64 {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(16))
	if o != 0 {
		return rcv._tab.GetInt64(o + rcv._tab.Pos)
	}
	return 0
}

func (rcv *Client) CPUUtilizationPeriod() int64 {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(18))
	if o != 0 {
		return rcv._tab.GetInt64(o + rcv._tab.Pos)
	}
	return 0
}

func ClientStart(builder *flatbuffers.Builder) { builder.StartObject(8) }
func ClientAddID(builder *flatbuffers.Builder, ID flatbuffers.UOffsetT) { builder.PrependUOffsetTSlot(0, flatbuffers.UOffsetT(ID), 0) }
func ClientStartIDVector(builder *flatbuffers.Builder, numElems int) flatbuffers.UOffsetT { return builder.StartVector(1, numElems, 1)
}
func ClientAddHostname(builder *flatbuffers.Builder, Hostname flatbuffers.UOffsetT) { builder.PrependUOffsetTSlot(1, flatbuffers.UOffsetT(Hostname), 0) }
func ClientAddRegion(builder *flatbuffers.Builder, Region flatbuffers.UOffsetT) { builder.PrependUOffsetTSlot(2, flatbuffers.UOffsetT(Region), 0) }
func ClientAddZone(builder *flatbuffers.Builder, Zone flatbuffers.UOffsetT) { builder.PrependUOffsetTSlot(3, flatbuffers.UOffsetT(Zone), 0) }
func ClientAddDataCenter(builder *flatbuffers.Builder, DataCenter flatbuffers.UOffsetT) { builder.PrependUOffsetTSlot(4, flatbuffers.UOffsetT(DataCenter), 0) }
func ClientAddMemInfoPeriod(builder *flatbuffers.Builder, MemInfoPeriod int64) { builder.PrependInt64Slot(5, MemInfoPeriod, 0) }
func ClientAddNetUsagePeriod(builder *flatbuffers.Builder, NetUsagePeriod int64) { builder.PrependInt64Slot(6, NetUsagePeriod, 0) }
func ClientAddCPUUtilizationPeriod(builder *flatbuffers.Builder, CPUUtilizationPeriod int64) { builder.PrependInt64Slot(7, CPUUtilizationPeriod, 0) }
func ClientEnd(builder *flatbuffers.Builder) flatbuffers.UOffsetT { return builder.EndObject() }
