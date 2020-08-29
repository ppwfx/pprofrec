package pprofrec

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/bits"
	"net/http"
	"os"
	"runtime"
	"runtime/pprof"
	"strconv"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/process"
)

type record struct {
	ts             time.Time
	memStats       runtime.MemStats
	pprofPair      pprofPair
	cpuTimeStat    cpu.TimesStat
	iOCounterStat  process.IOCountersStat
	memoryInfoStat process.MemoryInfoStat
}

type pprofPair struct {
	goroutine    int
	threadcreate int
	heap         int
	allocs       int
	block        int
	mutex        int
}

type capabilities struct {
	cpuTimeStat    bool
	iOCounterStat  bool
	memoryInfoStat bool
}

// WindowOpts configures the Window handler.
type WindowOpts struct {
	// Window defines a window within metrics are stored.
	Window time.Duration
	// Frequency defines at what frequency metrics are recorded.
	Frequency time.Duration
}

// Window records runtime metrics at a given frequency within a given window and
// responds with a html table that lists the recorded metrics.
func Window(ctx context.Context, opts WindowOpts) func(w http.ResponseWriter, r *http.Request) {
	if opts.Window == time.Duration(0) {
		opts.Window = 30 * time.Second
	}

	if opts.Frequency == time.Duration(0) {
		opts.Frequency = 1 * time.Second
	}

	var c capabilities
	p, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		log.Printf("pprofrec: failed to create process instance: %v", err.Error())
	} else {
		c = getCapabilities(ctx, p)
	}

	var rs []record
	go func() {
		max := int((opts.Window / opts.Frequency) + 1)
		for range time.Tick(opts.Frequency) {
			select {
			case <-ctx.Done():
				return
			default:
				if len(rs) < max {
					rs = append(rs, getRecord(ctx, c, p))
				} else {
					rs = append(rs[1:], getRecord(ctx, c, p))
				}
			}
		}
	}()

	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			err := r.Body.Close()
			if err != nil {
				log.Printf("pprofrec: failed to close request body: %v", err.Error())
			}
		}()

		w.Header().Set("Content-Type", "text/html; charset=UTF-8")

		err := writeHead(w, c)
		if err != nil {
			log.Printf("pprofrec: failed to write to response writer: %v", err.Error())

			return
		}

		switch {
		case len(rs) == 0:
			break
		case len(rs) == 1:
			err = writeRow(w, c, rs[0], rs[0])
			if err != nil {
				log.Printf("pprofrec: failed to write to response writer: %v", err.Error())
			}

			break
		default:
			err = writeRow(w, c, rs[0], rs[1])
			if err != nil {
				log.Printf("pprofrec: failed to write to response writer: %v", err.Error())
			}

			for i := 2; i < len(rs); i++ {
				err := writeRow(w, c, rs[i-1], rs[i])
				if err != nil {
					log.Printf("pprofrec: failed to write to response writer: %v", err.Error())
				}
			}
		}
	}
}

// WindowOpts configures the Window handler.
type StreamOpts struct {
	// Frequency defines at what frequency metrics are recorded and streamed.
	Frequency time.Duration
}

// Stream streams runtime metrics at a given frequency as a html table.
func Stream(opts StreamOpts) func(w http.ResponseWriter, r *http.Request) {
	if opts.Frequency == time.Duration(0) {
		opts.Frequency = 1 * time.Second
	}

	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			err := r.Body.Close()
			if err != nil {
				log.Printf("pprofrec: failed to close request body: %v", err.Error())
			}
		}()

		var c capabilities
		p, err := process.NewProcess(int32(os.Getpid()))
		if err != nil {
			log.Printf("pprofrec: failed to create process instance: %v", err.Error())
		} else {
			c = getCapabilities(r.Context(), p)
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=UTF-8")

		err = writeHead(w, c)
		if err != nil {
			log.Printf("pprofrec: failed to write to response writer: %v", err.Error())
		}
		flusher.Flush()

		previous := getRecord(r.Context(), c, p)
		var current record
		for range time.Tick(opts.Frequency) {
			select {
			case <-r.Context().Done():
				return
			default:
				current = getRecord(r.Context(), c, p)

				err = writeRow(w, c, previous, current)
				if err != nil {
					log.Printf("pprofrec: failed to write to response writer: %v", err.Error())
				}
				flusher.Flush()

				previous = current
			}
		}
	}
}

// getCapabilities determines what metrics are available on the current OS
func getCapabilities(ctx context.Context, p *process.Process) (c capabilities) {
	_, err := p.TimesWithContext(ctx)
	if err == nil || err.Error() != "not implemented yet" {
		c.cpuTimeStat = true
	}

	_, err = p.IOCountersWithContext(ctx)
	if err == nil || err.Error() != "not implemented yet" {
		c.iOCounterStat = true
	}

	_, err = p.MemoryInfoWithContext(ctx)
	if err == nil || err.Error() != "not implemented yet" {
		c.memoryInfoStat = true
	}

	return
}

// getRecords records a snapshot of the available metrics
func getRecord(ctx context.Context, c capabilities, p *process.Process) (r record) {
	r.ts = time.Now()

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	r.memStats = ms

	r.pprofPair = pprofPair{
		goroutine:    pprof.Lookup("goroutine").Count(),
		threadcreate: pprof.Lookup("threadcreate").Count(),
		heap:         pprof.Lookup("heap").Count(),
		allocs:       pprof.Lookup("allocs").Count(),
		block:        pprof.Lookup("block").Count(),
		mutex:        pprof.Lookup("mutex").Count(),
	}

	if c.cpuTimeStat {
		cpuTimeStat, err := p.TimesWithContext(ctx)
		if err != nil {
			log.Printf("pprofrec: failed to get cpu time stats: %s", err)
		}
		if cpuTimeStat != nil {
			r.cpuTimeStat = *cpuTimeStat
		} else {
			r.cpuTimeStat = cpu.TimesStat{}
		}
	}

	if c.iOCounterStat {
		iOCounterStat, err := p.IOCountersWithContext(ctx)
		if err != nil {
			log.Printf("pprofrec: failed to get io counter stats: %s", err)
		}
		if iOCounterStat != nil {
			r.iOCounterStat = *iOCounterStat
		} else {
			r.iOCounterStat = process.IOCountersStat{}
		}
	}

	if c.memoryInfoStat {
		memoryInfoStat, err := p.MemoryInfoWithContext(ctx)
		if err != nil {
			log.Printf("pprofrec: failed to get memory info stats: %s", err)
		}
		if memoryInfoStat != nil {
			r.memoryInfoStat = *memoryInfoStat
		} else {
			r.memoryInfoStat = process.MemoryInfoStat{}
		}
	}

	return
}

func writeHead(w io.Writer, c capabilities) (err error) {
	_, err = w.Write([]byte(`
<!DOCTYPE html>
<html>
<head>
	<style>
		body, table {
			font-family:Courier, monospace;
			font-size: 13px;
			white-space: nowrap;
			border-spacing: 0px;
			margin: 0px;
			padding: 0px;
		}

		table          { 
			overflow-y: auto; 
			height: 100px; 
		}

		table thead th { 
			background-color: white; 
			border-color: white;
			text-align: left;
		}

		table td { 
			padding-left: 5px; 
		}


		.tbl__head1 th {
			position: sticky;
			top: 0px;
			left: 69px;
			padding-left: 1px;
			background-color: white;
		}

		.tbl__head1__th1 { 
			left: 0px !important;
			z-index: 50;
			border-right: 1px solid gray;
		}

		.tbl__head2 th { 
			position: sticky; 
			top: 15px; 
			padding-bottom: 5px;
			border-bottom: 1px solid gray;
		}
		

		.tbl__th-time { 
			position: sticky;
			top: 0;
			left: 0;
			border-right: 1px solid gray;
			z-index: 20;
		}

		.tbl__col1 {
		  position: -webkit-sticky;
		  position: sticky;
		  background-color: white;
		  left: 0px;
		  padding-left: 0px;
		  padding-right: 5px;
		  font-weight: bold;
		  border-right: 1px solid gray;
		}
	</style>
	<title></title>
</head>
<body>
	<table>
			<thead class="tbl__head1">
				<th class="tbl__head1__th1" colspan="1"></th>`))
	if err != nil {
		return
	}

	_, err = w.Write([]byte(`<th colspan="12"><a target="_blank" href="https://godoc.org/runtime/pprof#Lookup">pprof.Lookup</a></th>`))
	if err != nil {
		return
	}

	_, err = w.Write([]byte(`<th colspan="52"><a target="_blank" href="https://godoc.org/runtime#MemStats">runtime.MemStats</a></th>`))
	if err != nil {
		return
	}

	if c.memoryInfoStat {
		_, err = w.Write([]byte(`<th colspan="14"><a target="_blank" href="https://godoc.org/github.com/shirou/gopsutil/process#MemoryInfoStat">process.MemoryInfoStat</a></th>`))
		if err != nil {
			return
		}
	}

	if c.cpuTimeStat {
		_, err = w.Write([]byte(`<th colspan="20"><a target="_blank" href="https://godoc.org/github.com/shirou/gopsutil/cpu#TimesStat">cpu.TimesStat</a></th>`))
		if err != nil {
			return
		}
	}

	if c.iOCounterStat {
		_, err = w.Write([]byte(`<th colspan="8"><a target="_blank" href="https://godoc.org/github.com/shirou/gopsutil/process#IOCountersStat">process.IOCountersStat</a></th>`))
		if err != nil {
			return
		}
	}

	_, err = w.Write([]byte(`</thead>
			<thead class="tbl__head2">
				<th class="tbl__th-time">time</th>`))
	if err != nil {
		return
	}

	err = writePprofTLookupMetricsHead(w)
	if err != nil {
		return
	}

	err = writeRuntimeMemStatsMetricsTHead(w)
	if err != nil {
		return
	}

	if c.memoryInfoStat {
		err = writeProcessMemoryInfoStatMetricsTHead(w)
		if err != nil {
			return
		}
	}

	if c.cpuTimeStat {
		err = writeProcessCpuTimesStatMetricsTHead(w)
		if err != nil {
			return
		}
	}

	if c.iOCounterStat {
		err = writeProcessIOCountersStatMetricsTHead(w)
		if err != nil {
			return
		}
	}

	_, err = w.Write([]byte(`</thead><tbody>`))
	if err != nil {
		return
	}

	return
}

func writePprofTLookupMetricsHead(w io.Writer) (err error) {
	_, err = w.Write([]byte(`<th colspan="2">goroutine</th>
<th colspan="2">threadcreate</th>
<th colspan="2">heap</th>
<th colspan="2">allocs</th>
<th colspan="2">block</th>
<th colspan="2">mutex</th>`))
	if err != nil {
		return
	}

	return
}

func writeProcessMemoryInfoStatMetricsTHead(w io.Writer) (err error) {
	_, err = w.Write([]byte(`<th colspan="2">.RSS</th>
<th colspan="2">.VMS</th>
<th colspan="2">.HWM</th>
<th colspan="2">.Data</th>
<th colspan="2">.Stack</th>
<th colspan="2">.Locked</th>
<th colspan="2">.Swap</th>`))
	if err != nil {
		return
	}

	return
}

func writeProcessIOCountersStatMetricsTHead(w io.Writer) (err error) {
	_, err = w.Write([]byte(`<th colspan="2">.ReadCount</th> 
<th colspan="2">.WriteCount</th>
<th colspan="2">.ReadBytes</th> 
<th colspan="2">.WriteBytes</th>`))
	if err != nil {
		return
	}

	return
}

func writeProcessCpuTimesStatMetricsTHead(w io.Writer) (err error) {
	_, err = w.Write([]byte(`<th colspan="2">.User</th>
<th colspan="2">.System</th>
<th colspan="2">.Idle</th>
<th colspan="2">.Nice</th>
<th colspan="2">.Iowait</th>
<th colspan="2">.Irq</th>
<th colspan="2">.Softirq</th>
<th colspan="2">.Steal</th>
<th colspan="2">.Guest</th>
<th colspan="2">.GuestNice</th>`))
	if err != nil {
		return
	}

	return
}

func writeRuntimeMemStatsMetricsTHead(w io.Writer) (err error) {
	_, err = w.Write([]byte(`<th colspan="2">.Alloc</th>
<th colspan="2">.TotalAlloc</th>
<th colspan="2">.Sys</th>
<th colspan="2">.Lookups</th>
<th colspan="2">.Mallocs</th>
<th colspan="2">.Frees</th>
<th colspan="2">.HeapAlloc</th>
<th colspan="2">.HeapSys</th>
<th colspan="2">.HeapIdle</th>
<th colspan="2">.HeapInuse</th>
<th colspan="2">.HeapReleased</th>
<th colspan="2">.HeapObjects</th>
<th colspan="2">.StackInuse</th>
<th colspan="2">.StackSys</th>
<th colspan="2">.MSpanInuse</th>
<th colspan="2">.MSpanSys</th>
<th colspan="2">.MCacheInuse</th>
<th colspan="2">.MCacheSys</th>
<th colspan="2">.BuckHashSys</th>
<th colspan="2">.GCSys</th>
<th colspan="2">.OtherSys</th>
<th colspan="2">.NextGC</th>
<th colspan="2">.LastGC</th>
<th colspan="2">.PauseTotalNs</th>
<th colspan="2">.NumGC</th>
<th colspan="2">.NumForcedGC</th>
<th colspan="2">.OtherSys</th>`))
	if err != nil {
		return
	}

	return
}

func writeRow(w io.Writer, c capabilities, previous record, current record) (err error) {
	_, err = w.Write([]byte(`<tr><td class="tbl__col1">`))
	if err != nil {
		return
	}

	_, err = w.Write([]byte(current.ts.Format("15:04:05")))
	if err != nil {
		return
	}

	err = writePprof(w, previous.pprofPair, current.pprofPair)
	if err != nil {
		return
	}

	err = writeMemStats(w, previous.memStats, current.memStats)
	if err != nil {
		return
	}

	if c.memoryInfoStat {
		err = writeMemoryInfoStat(w, previous.memoryInfoStat, current.memoryInfoStat)
		if err != nil {
			return
		}
	}

	if c.cpuTimeStat {
		err = writeCpuTimeStat(w, previous.cpuTimeStat, current.cpuTimeStat)
		if err != nil {
			return
		}
	}

	if c.iOCounterStat {
		err = writeIOCounterStat(w, previous.iOCounterStat, current.iOCounterStat)
		if err != nil {
			return
		}
	}

	_, err = w.Write([]byte("</td></tr>"))
	if err != nil {
		return
	}

	return
}

func writePprof(w io.Writer, previous pprofPair, current pprofPair) (err error) {
	err = writeIntCol(w, current.goroutine, current.goroutine-previous.goroutine)
	if err != nil {
		return
	}

	err = writeIntCol(w, current.threadcreate, current.threadcreate-previous.threadcreate)
	if err != nil {
		return
	}

	err = writeIntCol(w, current.heap, current.heap-previous.heap)
	if err != nil {
		return
	}

	err = writeIntCol(w, current.allocs, current.allocs-previous.allocs)
	if err != nil {
		return
	}

	err = writeIntCol(w, current.block, current.block-previous.block)
	if err != nil {
		return
	}

	err = writeIntCol(w, current.mutex, current.mutex-previous.mutex)
	if err != nil {
		return
	}

	return
}

func writeMemoryInfoStat(w io.Writer, previous process.MemoryInfoStat, current process.MemoryInfoStat) (err error) {
	err = writeBytesCol(w, current.RSS, int64(current.RSS-previous.RSS))
	if err != nil {
		return
	}

	err = writeBytesCol(w, current.VMS, int64(current.VMS-previous.VMS))
	if err != nil {
		return
	}

	err = writeBytesCol(w, current.HWM, int64(current.HWM-previous.HWM))
	if err != nil {
		return
	}

	err = writeBytesCol(w, current.Data, int64(current.Data-previous.Data))
	if err != nil {
		return
	}

	err = writeBytesCol(w, current.Stack, int64(current.Stack-previous.Stack))
	if err != nil {
		return
	}

	err = writeBytesCol(w, current.Locked, int64(current.Locked-previous.Locked))
	if err != nil {
		return
	}

	err = writeBytesCol(w, current.Swap, int64(current.Swap-previous.Swap))
	if err != nil {
		return
	}

	return
}

func writeIOCounterStat(w io.Writer, previous process.IOCountersStat, current process.IOCountersStat) (err error) {
	err = writeUint64Col(w, current.ReadCount, int64(current.ReadCount-previous.ReadCount))
	if err != nil {
		return
	}

	err = writeUint64Col(w, current.WriteCount, int64(current.WriteCount-previous.WriteCount))
	if err != nil {
		return
	}

	err = writeBytesCol(w, current.ReadBytes, int64(current.ReadBytes-previous.ReadBytes))
	if err != nil {
		return
	}

	err = writeBytesCol(w, current.WriteBytes, int64(current.WriteBytes-previous.WriteBytes))
	if err != nil {
		return
	}

	return
}

func writeCpuTimeStat(w io.Writer, previous cpu.TimesStat, current cpu.TimesStat) (err error) {
	err = writeDuration(w, time.Duration(current.User*float64(time.Second)), time.Duration((current.User-previous.User)*float64(time.Second)))
	if err != nil {
		return
	}

	err = writeDuration(w, time.Duration(current.System*float64(time.Second)), time.Duration((current.System-previous.System)*float64(time.Second)))
	if err != nil {
		return
	}

	err = writeDuration(w, time.Duration(current.Idle*float64(time.Second)), time.Duration((current.Idle-previous.Idle)*float64(time.Second)))
	if err != nil {
		return
	}

	err = writeDuration(w, time.Duration(current.Nice*float64(time.Second)), time.Duration((current.Nice-previous.Nice)*float64(time.Second)))
	if err != nil {
		return
	}

	err = writeDuration(w, time.Duration(current.Iowait*float64(time.Second)), time.Duration((current.Iowait-previous.Iowait)*float64(time.Second)))
	if err != nil {
		return
	}

	err = writeDuration(w, time.Duration(current.Irq*float64(time.Second)), time.Duration((current.Irq-previous.Irq)*float64(time.Second)))
	if err != nil {
		return
	}

	err = writeDuration(w, time.Duration(current.Softirq*float64(time.Second)), time.Duration((current.Softirq-previous.Softirq)*float64(time.Second)))
	if err != nil {
		return
	}

	err = writeDuration(w, time.Duration(current.Steal*float64(time.Second)), time.Duration((current.Steal-previous.Steal)*float64(time.Second)))
	if err != nil {
		return
	}

	err = writeDuration(w, time.Duration(current.Guest*float64(time.Second)), time.Duration((current.Guest-previous.Guest)*float64(time.Second)))
	if err != nil {
		return
	}

	err = writeDuration(w, time.Duration(current.GuestNice*float64(time.Second)), time.Duration((current.GuestNice-previous.GuestNice)*float64(time.Second)))
	if err != nil {
		return
	}

	return
}

func writeMemStats(w io.Writer, previous runtime.MemStats, current runtime.MemStats) (err error) {
	err = writeBytesCol(w, current.Alloc, int64(current.Alloc-previous.Alloc))
	if err != nil {
		return
	}

	err = writeBytesCol(w, current.TotalAlloc, int64(current.TotalAlloc-previous.TotalAlloc))
	if err != nil {
		return
	}

	err = writeBytesCol(w, current.Sys, int64(current.Sys-previous.Sys))
	if err != nil {
		return
	}

	err = writeUint64Col(w, current.Lookups, int64(current.Lookups-previous.Lookups))
	if err != nil {
		return
	}

	err = writeUint64Col(w, current.Mallocs, int64(current.Mallocs-previous.Mallocs))
	if err != nil {
		return
	}

	err = writeUint64Col(w, current.Frees, int64(current.Frees-previous.Frees))
	if err != nil {
		return
	}

	err = writeBytesCol(w, current.HeapAlloc, int64(current.HeapAlloc-previous.HeapAlloc))
	if err != nil {
		return
	}

	err = writeBytesCol(w, current.HeapSys, int64(current.HeapSys-previous.HeapSys))
	if err != nil {
		return
	}

	err = writeBytesCol(w, current.HeapIdle, int64(current.HeapIdle-previous.HeapIdle))
	if err != nil {
		return
	}

	err = writeBytesCol(w, current.HeapInuse, int64(current.HeapInuse-previous.HeapInuse))
	if err != nil {
		return
	}

	err = writeBytesCol(w, current.HeapReleased, int64(current.HeapReleased-previous.HeapReleased))
	if err != nil {
		return
	}

	err = writeUint64Col(w, current.HeapObjects, int64(current.HeapObjects-previous.HeapObjects))
	if err != nil {
		return
	}

	err = writeBytesCol(w, current.StackInuse, int64(current.StackInuse-previous.StackInuse))
	if err != nil {
		return
	}

	err = writeBytesCol(w, current.StackSys, int64(current.StackSys-previous.StackSys))
	if err != nil {
		return
	}

	err = writeBytesCol(w, current.MSpanInuse, int64(current.MSpanInuse-previous.MSpanInuse))
	if err != nil {
		return
	}

	err = writeBytesCol(w, current.MSpanSys, int64(current.MSpanSys-previous.MSpanSys))
	if err != nil {
		return
	}

	err = writeBytesCol(w, current.MCacheInuse, int64(current.MCacheInuse-previous.MCacheInuse))
	if err != nil {
		return
	}

	err = writeBytesCol(w, current.MCacheSys, int64(current.MCacheSys-previous.MCacheSys))
	if err != nil {
		return
	}

	err = writeBytesCol(w, current.BuckHashSys, int64(current.BuckHashSys-previous.BuckHashSys))
	if err != nil {
		return
	}

	err = writeBytesCol(w, current.GCSys, int64(current.GCSys-previous.GCSys))
	if err != nil {
		return
	}

	err = writeBytesCol(w, current.OtherSys, int64(current.OtherSys-previous.OtherSys))
	if err != nil {
		return
	}

	err = writeBytesCol(w, current.NextGC, int64(current.NextGC-previous.NextGC))
	if err != nil {
		return
	}

	err = writeTime(w, time.Unix(0, int64(current.LastGC)), time.Unix(0, int64(current.LastGC)).Sub(time.Unix(0, int64(previous.LastGC))))
	if err != nil {
		return
	}

	err = writeDuration(w, time.Duration(current.PauseTotalNs), time.Duration(current.PauseTotalNs-previous.PauseTotalNs))
	if err != nil {
		return
	}

	err = writeUint64Col(w, uint64(current.NumGC), int64(current.NumGC-previous.NumGC))
	if err != nil {
		return
	}

	err = writeUint64Col(w, uint64(current.NumForcedGC), int64(current.NumForcedGC-previous.NumForcedGC))
	if err != nil {
		return
	}

	err = writeBytesCol(w, current.OtherSys, int64(current.OtherSys-previous.OtherSys))
	if err != nil {
		return
	}

	return
}

func writeDuration(w io.Writer, value time.Duration, diff time.Duration) (err error) {
	_, err = w.Write([]byte("</td><td style=\"padding-left: 10px;\">"))
	if err != nil {
		return
	}

	_, err = w.Write([]byte(value.String()))
	if err != nil {
		return
	}

	switch {
	case diff > 0:
		_, err = w.Write([]byte(`</td><td style="color: green;">`))
		if err != nil {
			return
		}
	case diff < 0:
		_, err = w.Write([]byte(`</td><td style="color: red;">`))
		if err != nil {
			return
		}
	case diff == 0:
		_, err = w.Write([]byte(`</td><td style="color: gray;">`))
		if err != nil {
			return
		}
	}

	_, err = w.Write([]byte(diff.String()))
	if err != nil {
		return
	}

	return
}

func writeTime(w io.Writer, value time.Time, diff time.Duration) (err error) {
	_, err = w.Write([]byte("</td><td style=\"padding-left: 10px;\">"))
	if err != nil {
		return
	}

	_, err = w.Write([]byte(value.Format("15:04:05.000000000")))
	if err != nil {
		return
	}

	switch {
	case diff > 0:
		_, err = w.Write([]byte(`</td><td style="color: green;">`))
		if err != nil {
			return
		}
	case diff < 0:
		_, err = w.Write([]byte(`</td><td style="color: red;">`))
		if err != nil {
			return
		}
	case diff == 0:
		_, err = w.Write([]byte(`</td><td style="color: gray;">`))
		if err != nil {
			return
		}
	}

	_, err = w.Write([]byte(diff.String()))
	if err != nil {
		return
	}

	return
}

func writeIntCol(w io.Writer, v int, diff int) (err error) {
	_, err = w.Write([]byte("</td><td style=\"padding-left: 10px;\">"))
	if err != nil {
		return
	}

	_, err = w.Write([]byte(strconv.FormatInt(int64(v), 10)))
	if err != nil {
		return
	}

	switch {
	case diff > 0:
		_, err = w.Write([]byte(`</td><td style="color: green;">`))
		if err != nil {
			return
		}
	case diff < 0:
		_, err = w.Write([]byte(`</td><td style="color: red;">`))
		if err != nil {
			return
		}
	case diff == 0:
		_, err = w.Write([]byte(`</td><td style="color: gray;">`))
		if err != nil {
			return
		}
	}

	_, err = w.Write([]byte(strconv.FormatInt(int64(diff), 10)))
	if err != nil {
		return
	}

	return
}

func writeUint64Col(w io.Writer, v uint64, diff int64) (err error) {
	_, err = w.Write([]byte("</td><td style=\"padding-left: 10px;\">"))
	if err != nil {
		return
	}

	_, err = w.Write([]byte(strconv.FormatUint(v, 10)))
	if err != nil {
		return
	}

	switch {
	case diff > 0:
		_, err = w.Write([]byte(`</td><td style="color: green;">`))
		if err != nil {
			return
		}
	case diff < 0:
		_, err = w.Write([]byte(`</td><td style="color: red;">`))
		if err != nil {
			return
		}
	case diff == 0:
		_, err = w.Write([]byte(`</td><td style="color: gray;">`))
		if err != nil {
			return
		}
	}

	_, err = w.Write([]byte(strconv.FormatInt(diff, 10)))
	if err != nil {
		return
	}

	return
}

func writeBytesCol(w io.Writer, v uint64, diff int64) (err error) {
	_, err = w.Write([]byte("</td><td style=\"padding-left: 10px;\">"))
	if err != nil {
		return
	}

	_, err = writeHumanBytes(w, int64(v))
	if err != nil {
		return
	}

	switch {
	case diff > 0:
		_, err = w.Write([]byte(`</td><td style="color: green;">`))
		if err != nil {
			return
		}
	case diff < 0:
		_, err = w.Write([]byte(`</td><td style="color: red;">`))
		if err != nil {
			return
		}
	case diff == 0:
		_, err = w.Write([]byte(`</td><td style="color: gray;">`))
		if err != nil {
			return
		}
	}

	_, err = writeHumanBytes(w, diff)
	if err != nil {
		return
	}

	return
}

func writeHumanBytes(w io.Writer, bytes int64) (n int, err error) {
	var abs uint64
	if bytes < 0 {
		abs = uint64(-bytes)
	} else {
		abs = uint64(bytes)
	}

	if abs < 1024 {
		return fmt.Fprintf(w, "%d B", bytes)
	}

	base := uint(bits.Len64(abs) / 10)
	val := float64(bytes) / float64(uint64(1<<(base*10)))

	return fmt.Fprintf(w, "%.3f %ciB", val, " KMGTPE"[base])
}
