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
)

var (
	cfg       *amqpconfig.Config
	wg        sync.WaitGroup
	listeners []*types.AMQPProtocol
)

func main() {

	var p *amqp1.Protocol
	var err error
	var opts []amqp1.Option

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
					fmt.Printf("Error marshalling event data %v", err)
				}
				/*if data.ID == 1 {
					currentBatchMaxDiff = 0
					msgCurrentBatchCount = 0
				}*/

				diff := time.Since(e.Context.GetTime()).Microseconds()
				if diff > l.MaxDiff {
					l.MaxDiff = diff
				}
				/*if diff > currentBatchMaxDiff {
					currentBatchMaxDiff = diff
				}*/

				atomic.AddUint64(&l.MsgReceivedCount, 1)
				//atomic.AddUint64(&msgCurrentBatchCount, 1)
				if (int(l.MsgReceivedCount) % cfg.MsgCount) == 0 {
					fmt.Printf("\n CE-AMQP: Total message recived for queue %s = %d, maxDiff = %d\n", l.ID, l.MsgReceivedCount, l.MaxDiff)
					//fmt.Printf("CE-AMQP: Total current batch message received for queue %s = %d, maxDiff = %d\n", msgCurrentBatchCount, l.Queue, currentBatchMaxDiff)
				}

			})
			if err != nil {
				log.Printf("AMQP receiver error: %v", err)
			}

		}(l)

	}

	wg.Wait()

	log.Print("End Consumer")
}
