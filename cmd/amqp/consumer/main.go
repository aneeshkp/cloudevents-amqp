package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Azure/go-amqp"
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
	"log"
	"sync"
	"sync/atomic"
	"time"

	amqpconfig "github.com/aneeshkp/cloudevents-amqp/pkg/config/amqp"
	amqp1 "github.com/cloudevents/sdk-go/protocol/amqp/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
)

const (
	defaultMsgCount = 10
	maxDiffDefault  = 0
	minDiffDefault  = 99999999999999
)

var (
	cfg       *amqpconfig.Config
	wg        sync.WaitGroup
	listeners []*types.AMQPProtocol
	// Pool for  struct Result
	pool *sync.Pool
)

// Func to init pool
func initResultPool() {
	pool = &sync.Pool {
		New: func()interface{} {
			return new(types.Result)
		},
	}
}


func main() {

	var p *amqp1.Protocol
	var err error
	var opts []amqp1.Option
	initResultPool()

	opts = append(opts, amqp1.WithReceiverLinkOption(amqp.LinkCredit(50)))

	cfg, err = amqpconfig.GetConfig()
	if err != nil {
		log.Printf("Could not load configuration file --config, loading default queue%v\n", err)
		cfg = &amqpconfig.Config{
			TimeOut:  5,
			MsgCount: defaultMsgCount,
			HostName: "amqp://localhost",
			Port:     5672,
			Listener: amqpconfig.Listener{
				Queue: []amqpconfig.Queue{
					{
						Name:  "test/node1",
						Count: 2,
					},
				},
			},
		}
	}
	log.Printf("Connecting to host %s:%d", cfg.HostName, cfg.Port)
	for _, q := range cfg.Listener.Queue {
		for i := 1; i <= q.Count; i++ {
			l := types.AMQPProtocol{}
			l.Queue = q.Name
			l.MaxDiff = maxDiffDefault
			l.MinDiff = minDiffDefault
			l.MaxDiff2 = maxDiffDefault
			l.MinDiff2 = minDiffDefault
			l.ID = fmt.Sprintf("%s-#%d", l.Queue, i)
			for {
				p, err = amqp1.NewProtocol2(fmt.Sprintf("%s:%d", cfg.HostName, cfg.Port), "", l.Queue, []amqp.ConnOption{}, []amqp.SessionOption{}, opts...)
				if err != nil {
					log.Printf("Failed to create amqp protocol (trying in 5 secs): %v", err)
					time.Sleep(5 * time.Second)
				} else {
					log.Printf("Connection established for consumer %s\n", l.ID)
					break
				}
			}
			l.Protocol = p
			l.Ctx = context.Background()
			defer l.Protocol.Close(context.Background())
			c, err := cloudevents.NewClient(p)
			if err != nil {
				log.Fatalf("Failed to create client: %v", err)
			}

			l.Client = c
			listeners = append(listeners, &l)
		}
	}

	for _, l := range listeners {
		wg.Add(1)
		go func(l *types.AMQPProtocol) {
			fmt.Printf("listenining to queue %s by %s\n", l.Queue, l.ID)
			defer wg.Done()
			err = l.Client.StartReceiver(context.Background(), func(e cloudevents.Event) {
				data := types.Message{}
				err := json.Unmarshal(e.Data(), &data)
				if err != nil {
					log.Printf("Error marshalling event data %v", err)
				}
				atomic.AddUint64(&l.MsgReceivedCount, 1)
				// time stamped at the cnf
				atSourceDiff:=time.Since(data.GetTime()).Microseconds()
				// current max
				if atSourceDiff>l.CurrentMax{
					l.CurrentMax=atSourceDiff
				}
				// max diff
				if atSourceDiff > l.MaxDiff {
					l.MaxDiff = atSourceDiff
				}
				// min diff
				if atSourceDiff < l.MinDiff {
					l.MinDiff = atSourceDiff
				}
				// time stamped at the side car
				atSideCarDiff := time.Since(e.Context.GetTime()).Microseconds()
				if atSideCarDiff > l.MaxDiff2 {
					l.MaxDiff2 = atSideCarDiff
				}
				if atSideCarDiff < l.MinDiff2 {
					l.MinDiff2 = atSideCarDiff
				}
				// current max
				if atSideCarDiff>l.CurrentMax2{
					l.CurrentMax2=atSourceDiff
				}
				if (l.MsgReceivedCount % defaultMsgCount) == 0 {
					l.CurrentMax=0
					l.CurrentMax2=0
				}

			})
			if err != nil {
				log.Printf("AMQP receiver error: %v", err)
			}

		}(l)

	}
	wg.Add(1)
	go func(l []*types.AMQPProtocol) {
		defer wg.Done()

		var donotprintcount=0
		uptimeTicker := time.NewTicker(2 * time.Second)
		for { //nolint:gosimple
			select {
			case <-uptimeTicker.C:
				donotprintcount++
				if donotprintcount>5 {
					for _, l := range listeners {

						result := pool.Get().(*types.Result)
						result.Write(*l)
						log.Printf("ID\t\t\tMsg Received\t\tMax source\t\tMax sidecar\t\tMin source\t\tMin sidecar\n\t\tCurent Max\t\t sidecar current max")
						log.Printf("---------------------------------------------------------------------------------------------------------------------------------\n")
						log.Printf("%s\t\t%d\t\t%d\t\t\t%d\t\t\t%d\t\t\t%d\t\t%d\t\t%d\n",
							result.ID, result.MsgReceivedCount, result.FromSourceMaxDiff,
							result.FromSideCarMaxDiff, result.FromSourceMinDiff,
							result.FromSideCarMinDiff, result.FromSourceCurrentMax, result.FromSideCarCurrentMax)
						pool.Put(result)

					}
				}
			}
		}
	}(listeners)

	wg.Wait()

	log.Print("End Consumer")
}
