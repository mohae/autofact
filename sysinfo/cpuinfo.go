package sysinfo

// requires sysData for cpuDatas

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/google/flatbuffers/go"
)

const nl = '\n'

// CPUDatas contains the information that is generated by mpData.
// TODO: should the CPUData based stuff be elided?  Most usage should be
// flatbuffer based.
type CPUDatas struct {
	Timestamp string
	CPUID     string
	Usr       int16
	Nice      int16
	Sys       int16
	IOWait    int16
	IRQ       int16
	Soft      int16
	Steal     int16
	Guest     int16
	GNice     int16
	Idle      int16
}

func (c CPUDatas) String() string {
	return fmt.Sprintf("%s\t%s\t%0.2f\t%0.2f\t%0.2f\t%0.2f\t%0.2f\t%0.2f\t%0.2f\t%0.2f\t%0.2f\t%0.2f\n", c.Timestamp, c.CPUID, float32(c.Usr)/100.0, float32(c.Nice)/100.0, float32(c.Sys)/100.0, float32(c.IOWait)/100.0, float32(c.IRQ)/100.0, float32(c.Soft)/100.0, float32(c.Steal)/100.0, float32(c.Guest)/100.0, float32(c.GNice)/100.0, float32(c.Idle)/100.0)
}

// CheckCPUDatas gathers the CPUDatas for all CPUs using mpData.
func CheckCPUDatas() ([]CPUDatas, error) {
	var out bytes.Buffer
	cmd := exec.Command("mpstat", "-P", "ALL")
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting cpu stats: %s\n", err)
		return nil, err
	}
	var x, i int
	var cpuDatas []CPUDatas
	// process the output
	for {
		bs, err := out.ReadBytes(nl)
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Printf("error reading bytes from command execution: %s\n", err)
			return nil, err
		}
		x++
		// skip the first 4 lines
		if x < 4 {
			continue
		}
		// skip empty lines
		if len(bs) == 0 {
			continue
		}
		var cpuData CPUDatas
		cpuData.Timestamp = string(bs[:11])
		// tmp holds the current field
		tmp := make([]byte, 5)
		// ndx is the counter into tmp
		// fieldNum is used to match up the current value to its struct
		// field; the number does not translate to the ndx
		var ndx, fieldNum int
		for _, v := range bs[12:] {
			// 0x20 separates fields, there can be consecutive 0x20
			// occurrences for proper output alignment (when displayed)
			if v == 0x20 {
				// if something has been saved to tmp, it needs to be
				// written to the appropriate field
				if ndx > 0 {
					// CPUID is a string, so don't try to convert to int.
					// convert everything else.
					if fieldNum > 0 {
						i, _ = strconv.Atoi(string(tmp[:ndx]))
					}
					switch fieldNum {
					case 0:
						cpuData.CPUID = string(tmp[:ndx])
					case 1:
						cpuData.Usr = int16(i)
					case 2:
						cpuData.Nice = int16(i)
					case 3:
						cpuData.Sys = int16(i)
					case 4:
						cpuData.IOWait = int16(i)
					case 5:
						cpuData.IRQ = int16(i)
					case 6:
						cpuData.Soft = int16(i)
					case 7:
						cpuData.Steal = int16(i)
					case 8:
						cpuData.Guest = int16(i)
					case 9:
						cpuData.GNice = int16(i)
					}
					fieldNum++
				}
				// reset for the next field
				ndx = 0
				continue
			}
			// skip . and nl
			if v == 0x2E || v == nl {
				continue
			}
			tmp[ndx] = v
			ndx++
		}
		// the last element hasn't been saved, do it here
		i, err := strconv.Atoi(string(tmp[:ndx]))
		cpuData.Idle = int16(i)
		cpuDatas = append(cpuDatas, cpuData)
	}
	return cpuDatas, nil
}

// CPUDataTicker gets the CPU Datas on a ticker and outputs each cpu's
// Data as a flatbuffer encoded []byte. The 'all' CPU record will be
// discarded.  Each CPU's data is emitted separately.
func CPUDataTicker(interval time.Duration, outCh chan []byte) {
	ticker := time.NewTicker(interval)
	defer close(outCh)
	defer ticker.Stop()
	var out bytes.Buffer
	// pos is the current position in buffer
	// ndx is the pointer to the current byte in fld.
	// fldNum is the current struct field being processed.  This index does not
	// match up with the actual struct field.
	var pos, ndx, fldNum int
	// i is a tmp int; line is a counter of the command output lines
	var i, line int
	// fld holds the current field, which may be a max of 5 bytes
	fld := make([]byte, 5)
	builder := flatbuffers.NewBuilder(0)
	for {
		select {
		case <-ticker.C:
			fmt.Println("ticker: gathering cpu stats")
			cmd := exec.Command("mpstat", "-P", "ALL")
			cmd.Stdout = &out
			err := cmd.Run()
			if err != nil {
				fmt.Fprintf(os.Stderr, "error getting cpu stats: %s\n", err)
				return
			}
			t := time.Now().UTC().UnixNano()
			// process the output
			for {
				bs, err := out.ReadBytes(nl)
				if err != nil {
					if err == io.EOF {
						break
					}
					fmt.Fprintf(os.Stderr, "error reading bytes from command execution: %s\n", err)
					return
				}
				// skip the first 5 lines (this includes the all output)
				line++
				// TODO: is this true on all nix systems?
				if line < 5 {
					continue
				}
				// skip empty lines
				if len(bs) == 0 {
					continue
				}
				// Need to process the cpuid first because it's a string.
				// everything prior to pos12 is the timestamp but we are using
				// the timestamp obtained right before command execution for
				// better time resolution and because the target DB expects
				// an UTC timestamp instead of a string.
				for i, v := range bs[12:] {
					if v == 0x20 {
						continue
					}
					pos = 12 + i
					break
				}

				// get the id
				for i, v := range bs[pos:] {
					if v == 0x20 {
						pos += i
						break
					}
					fld[ndx] = v
					ndx++
				}
				id := builder.CreateByteString(fld[:ndx])
				ndx = 0
				CPUDataStart(builder)
				CPUDataAddCPUID(builder, id)
				CPUDataAddTimestamp(builder, t)
				// ndx is the counter into tmp
				// fieldNum is used to match up the current value to its struct
				// field; the number does not translate to the ndx
				for _, v := range bs[12:] {
					// 0x20 separates fields, there can be consecutive 0x20
					// occurrences for proper output alignment (when displayed)
					if v == 0x20 {
						// if something has been saved to tmp, it needs to be
						// written to the appropriate field
						if ndx > 0 {
							// CPUID is a string, so don't try to convert to int.
							// convert everything else.
							i, _ = strconv.Atoi(string(fld[:ndx]))
							switch fldNum {
							case 0:
								CPUDataAddUsr(builder, int16(i))
							case 1:
								CPUDataAddNice(builder, int16(i))
							case 2:
								CPUDataAddSys(builder, int16(i))
							case 3:
								CPUDataAddIOWait(builder, int16(i))
							case 4:
								CPUDataAddIRQ(builder, int16(i))
							case 5:
								CPUDataAddSoft(builder, int16(i))
							case 6:
								CPUDataAddSteal(builder, int16(i))
							case 7:
								CPUDataAddGuest(builder, int16(i))
							case 8:
								CPUDataAddGNice(builder, int16(i))
							}
							fldNum++
						}
						// reset for the next field
						ndx = 0
						continue
					}
					// skip . and nl
					if v == 0x2E || v == nl {
						continue
					}
					fld[ndx] = v
					ndx++
				}
				// the last element hasn't been saved, do it here
				i, _ = strconv.Atoi(string(fld[:ndx]))
				CPUDataAddIdle(builder, int16(i))
				builder.Finish(CPUDataEnd(builder))
				// send the bytes
				tmp := builder.Bytes[builder.Head():]
				// copy the Bytes
				cpy := make([]byte, len(tmp))
				copy(cpy, tmp)
				outCh <- cpy
				// reset the builder
				builder.Reset()
				ndx, fldNum = 0, 0
			}
			// reset the output buffer
			out.Reset()
			line = 0
		}
	}
}

// UnmarshalCPUDataToString takes a flatbuffers serialized []byte and returns
// the bytes as a formatted string.
func UnmarshalCPUDataToString(p []byte) string {
	c := GetRootAsCPUData(p, 0)
	return fmt.Sprintf("%d\t%s\t%0.2f\t%0.2f\t%0.2f\t%0.2f\t%0.2f\t%0.2f\t%0.2f\t%0.2f\t%0.2f\t%0.2f\n", c.Timestamp(), string(c.CPUID()), float32(c.Usr())/100.0, float32(c.Nice())/100.0, float32(c.Sys())/100.0, float32(c.IOWait())/100.0, float32(c.IRQ())/100.0, float32(c.Soft())/100.0, float32(c.Steal())/100.0, float32(c.Guest())/100.0, float32(c.GNice())/100.0, float32(c.Idle())/100.0)
}
