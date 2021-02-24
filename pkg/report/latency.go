package report

import (
	"fmt"
	"sync"
	"time"
)

const (
	maxBinSize = 1002
)

// Latency stores time in ms for each event received
type Latency struct {
	Time int64
}

// LatencyReport struct is used to collect latency report
type LatencyReport struct {
	id               string
	latency          [maxBinSize]int64
	msgReceivedCount int64
	wg               *sync.WaitGroup
	print            bool
}

// New ... create new LatencyReport object
func New(wg *sync.WaitGroup, ID string) *LatencyReport {
	return &LatencyReport{
		id:               ID,
		latency:          [1002]int64{},
		msgReceivedCount: 0,
		wg:               wg,
		print:            false,
	}
}

// Collect collects latency information and prints it
func (l *LatencyReport) Collect(msg <-chan int64) {
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		for { //nolint:gosimple
			select { //nolint:gosimple
			case latency := <-msg:
				l.print = true
				if latency >= int64(maxBinSize) {
					latency = int64(maxBinSize - 1)
				}
				l.latency[latency]++
				l.msgReceivedCount++
			}
		}
	}()
	l.printReport()
}
func (l *LatencyReport) printReport() {
	l.wg.Add(1)
	go func(l *LatencyReport) {
		defer l.wg.Done()
		uptimeTicker := time.NewTicker(5 * time.Second)
		for { //nolint:gosimple
			select {
			case <-uptimeTicker.C:
				if !l.print {
					continue
				}
				fmt.Printf("|%-15s|%15s|%15s|%15s|%15s|", "ID", "Total Msg", "Latency(ms)", "Msg", "Histogram(%)")
				fmt.Println()
				var j int64
				for i := 0; i < maxBinSize; i++ {
					latency := l.latency[i]
					li := float64(l.msgReceivedCount)
					if latency > 0 {
						fmt.Printf("|%-15s|%15d|%15d|%15d|", l.id, l.msgReceivedCount, i, latency)
						//calculate percentage
						var lf float64
						if latency == 0 {
							lf = 0.9
						} else {
							lf = float64(latency)
						}
						percent := (100 * lf) / li
						fmt.Printf("%10s%.2f%c|", "", percent, '%')
						for j = 1; j <= int64(percent); j++ {
							fmt.Printf("%c", '∎')
						}
						fmt.Println()
					}
				}
				fmt.Println()
			}
		}
	}(l)
}
