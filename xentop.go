// Go wrapper of the xentop utility.
package xentop

import (
	"bufio"
	"fmt"
	"os/exec"
	"reflect"
	"runtime"
	"strconv"
	"strings"
)

// A line in xentop
type Line struct {
	Name               string  `field:"NAME"`
	State              string  `field:"STATE"`
	CpuTime            int64   `field:"CPU(sec)"`
	CpuFraction        float32 `field:"CPU(%)"`
	Memory             int64   `field:"MEM(k)"`
	MaxMemory          int64   `field:"MAXMEM(k)"`
	MemoryFraction     float32 `field:"MEM(%)"`
	MaxMemoryFraction  float32 `field:"MAXMEM(%)"`
	VirtualCpus        int64   `field:"VCPUS"`
	NetworkInterfaces  int64   `field:"NETS"`
	NetworkTx          int64   `field:"NETTX(k)"`
	NetworkRx          int64   `field:"NETRX(k)"`
	VirtualDisks       int64   `field:"VBDS"`
	DiskBlockedIO      int64   `field:"VBD_OO"`
	DiskReadOps        int64   `field:"VBD_RD"`
	DiskWriteOps       int64   `field:"VBD_WR"`
	DiskSectorsRead    int64   `field:"VBD_RSECT"`
	DiskSectorsWritten int64   `field:"VBD_WSECT"`
	SSID               int64   `field:"SSID"`
}

// Fills a Line struct with the values from parseLine
func fillLine(data map[string]string) (ret Line, errs []error) {
	errs = []error{}
	pRet := &Line{}
	sv := reflect.Indirect(reflect.ValueOf(pRet))
	st := sv.Type()
	for i := 0; i < st.NumField(); i++ {
		fieldType := st.Field(i)
		fieldName, ok := fieldType.Tag.Lookup("field")
		if !ok {
			continue
		}
		val, ok := data[fieldName]
		if !ok {
			errs = append(errs, fmt.Errorf("Missing field  %s", fieldName))
			continue
		}
		delete(data, fieldName)
		field := sv.FieldByIndex(fieldType.Index)
		if val == "n/a" || val == "no-limit" {
			continue
		}
		switch fieldType.Type.Kind() {
		case reflect.String:
			field.SetString(val)
		case reflect.Float32:
			pVal, err := strconv.ParseFloat(val, 32)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: could not parse: %s", fieldName, err))
				continue
			}
			field.SetFloat(float64(pVal))
		case reflect.Int64:
			pVal, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: could not parse: %s", fieldName, err))
				continue
			}
			field.SetInt(pVal)
		default:
			panic("Encountered unexpected fieldtype in Line struct")
		}
	}
	ret = *pRet
	return
}

func reverse(lst []string) chan string {
    ret := make(chan string)
    go func() {
        for i, _ := range lst {
            ret <- lst[len(lst)-1-i]
        }
        close(ret)
    }()
    return ret
}

// Parse a line returned by "xentop -b"
func parseLine(line string, header []string) (map[string]string, error) {

	ret := make(map[string]string)
	line = strings.Replace(line, "no limit", "no-limit", -1) // avoid spaces in fields
	fields := strings.Fields(line)

	i := 0
	for key := range reverse(header) {
		ret[key] = fields[len(fields) - 1 - i]

		i = i + 1
		if i == len(header) {
			name := strings.Join(fields[0:len(fields) - i + 1], "-")
			ret[key] = name
		}
	}
	return ret, nil

func XenTopCmd(lines chan<- Line, errs chan<- error, cmdPath string) {
	cmd := exec.Command(cmdPath, "-b")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		errs <- fmt.Errorf("fatal: %s", err)
		return
	}
	defer stdout.Close()

	if err := cmd.Start(); err != nil {
		errs <- fmt.Errorf("fatal: %s", err)
		return
	}

	r := bufio.NewReader(stdout)

	var header []string

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			errs <- fmt.Errorf("fatal: %s", err)
			return
		}
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "NAME") {
			header = strings.Fields(line)
			continue
		}

		if header == nil {
			errs <- fmt.Errorf("Missing header")
			return
		}

		fields, err := parseLine(line, header)
		if err != nil {
			errs <- err
		}
		pLine, pErrs := fillLine(fields)

		// Sometimes xentop reports a ridiculously high CPU time and CPU
		// which should not be trusted and also breaks alignment of the other
		// fields in this line.  If we notice this, we simply ignore the entire
		// line.
		if pLine.CpuFraction > float32(runtime.NumCPU()*200) {
			errs <- fmt.Errorf("Crazy CPU(%%) value (%f) --- ignoring line",
				pLine.CpuFraction)
			continue
		}

		if len(pErrs) != 0 {
			errs <- fmt.Errorf("Couldn't parse %v: found error(s): %v",
				line, pErrs)
		}

		lines <- pLine
	}
}

// Runs xentop and writes lines and errors back over the provided channels.
func XenTop(lines chan<- Line, errs chan<- error) {
	XenTopCmd(lines, errs, "xentop")
}
