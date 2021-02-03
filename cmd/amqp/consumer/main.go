package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Azure/go-amqp"
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	amqpconfig "github.com/aneeshkp/cloudevents-amqp/pkg/config/amqp"
	amqp1 "github.com/cloudevents/sdk-go/protocol/amqp/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
)

const (
	defaultMsgCount         = 10
	maxBinSize              = 1002
	connectionRetryDuration = 5
	//bufferSize              = 100
)

var (
	cfg       *amqpconfig.Config
	wg        sync.WaitGroup
	listeners []*types.AMQPProtocol
	//latencyChan            chan MsgLatency
	latencyResult  map[string]*latency
	latencyResults []int
	//channelBufferSize      = bufferSize
	binSize                = maxBinSize
	globalMsgReceivedCount int64
	useSDK                 bool = true
)

type latency struct {
	ID       string
	MsgCount int64
	Latency  [maxBinSize]int64
}

//MsgLatency ... This for channel type to pass messages
/*type MsgLatency struct {
	ID       string
	MsgCount int64
	Latency  int64
}*/

func main() {

	var p *amqp1.Protocol
	var err error
	var opts []amqp1.Option
	//. Create by new
	envBinSize := os.Getenv("BIN_SIZE")
	if envBinSize != "" {
		binSize, err = strconv.Atoi(envBinSize)
		if err != nil {
			log.Fatalf("failed to read env `BIN_SIZE` %v", err)
		}
	}
	envUseSdk := os.Getenv("USE_SDK")
	if envUseSdk != "" {
		useSDK, err = strconv.ParseBool(envUseSdk)
		if err != nil {
			log.Fatalf("failed to read env `USE_SDK` %v", err)
		}
	}

	//latencyChan = make(chan MsgLatency, channelBufferSize)

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
						Count: 1,
					},
				},
			},
		}
	}

	log.Printf("Connecting to host %s:%d", cfg.HostName, cfg.Port)
	latencyResult = make(map[string]*latency, len(cfg.Listener.Queue))
	latencyResults = make([]int, binSize)

	if !useSDK {
		fmt.Println("Starting without cloud events SDK")
		wg.Add(1)
		go func(latencyResult map[string]*latency, wg *sync.WaitGroup) {
			defer wg.Done()
			nakedAmqp(fmt.Sprintf("%s/%d", cfg.HostName, cfg.Port), cfg.Listener.Queue[0].Name)
		}(latencyResult, &wg)

	} else {
		fmt.Println("Starting with cloud events SDK")
		for _, q := range cfg.Listener.Queue {
			l := types.AMQPProtocol{}
			l.ID = q.Name
			lr := latency{
				ID:       l.ID,
				MsgCount: 0,
			}
			latencyResult[l.ID] = &lr
			l.Queue = q.Name

			for {
				p, err = amqp1.NewProtocol2(fmt.Sprintf("%s:%d", cfg.HostName, cfg.Port), "", l.Queue, []amqp.ConnOption{}, []amqp.SessionOption{}, opts...)
				if err != nil {
					log.Printf("Failed to create amqp protocol (trying in 5 secs): %v", err)
					time.Sleep(connectionRetryDuration * time.Second)
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

		for _, l := range listeners {
			wg.Add(1)
			go func(l *types.AMQPProtocol, wg *sync.WaitGroup) {
				fmt.Printf("listenining to queue %s by %s\n", l.Queue, l.ID)
				defer wg.Done()
				err = l.Client.StartReceiver(context.Background(), func(e cloudevents.Event) {
					data := types.Message{}
					err := json.Unmarshal(e.Data(), &data)
					if err != nil {
						log.Printf("Error marshalling event data %v", err)
					}
					//atomic.AddUint64(&l.MsgReceivedCount, 1)
					// time stamped at the cnf
					atSourceDiff := time.Since(data.GetTime()).Milliseconds()
					//anything above 100ms is considered 100 ms
					if atSourceDiff >= int64(binSize) {
						atSourceDiff = int64(binSize)
					}
					/*r := MsgLatency{
						ID:       l.ID,
						MsgCount: int64(l.MsgReceivedCount),
						Latency:  atSourceDiff,
					}*/
					//latencyChan <- r
					latencyResults[atSourceDiff]++
					globalMsgReceivedCount++
					/*ll := latencyResult[l.ID]
					ll.Latency[atSourceDiff]++
					ll.MsgCount =int64(l.MsgReceivedCount)*/
				})
				if err != nil {
					log.Printf("AMQP receiver error: %v", err)
				}

			}(l, &wg)

		}
	} //else
	// Update latencyResult map by incrementing bin
	/*wg.Add(1)
	go func(latencyResult map[string]*latency, latencyBin chan MsgLatency, wg *sync.WaitGroup) {
		defer wg.Done()
		for l := range latencyBin {
			ll := latencyResult[l.ID]
			ll.Latency[l.Latency]++
			ll.MsgCount = l.MsgCount
		}
	}(latencyResult, latencyChan, &wg)*/

	// print result

	wg.Add(1)
	//go func(l []*types.AMQPProtocol,wg *sync.WaitGroup,) {

	go func(latencyResult map[string]*latency, wg *sync.WaitGroup,q string) {
		defer wg.Done()
		uptimeTicker := time.NewTicker(5 * time.Second)

		for { //nolint:gosimple
			select {
			case <-uptimeTicker.C:
				fmt.Printf("|%-15s|%15s|%15s|%15s|", "ID", "Msg Count", "Latency(ms)", "Histogram(%)")
				fmt.Println()

				var j int64
				for i := 0; i < binSize; i++ {
					if latencyResults[i] > 0 {
						fmt.Printf("%-15s%15d%15d%15d", q, globalMsgReceivedCount, i, latencyResults[i])
						//calculate percentage
						lf := float64(latencyResults[i])
						li := float64(globalMsgReceivedCount)
						percent := (100 * lf) / li
						fmt.Printf("%2.2f%c", percent, '%')
						for j = 1; j <= int64(percent); j++ {
							fmt.Printf("%c", '∎')
						}
						fmt.Println()
					}
				}
				fmt.Println()

			}

			/*for _, l := range listeners {
				result := pool.Get().(*types.Result)
				result.Write(*l)
				log.Printf("ID\t\t\tMsg Received\t\tMax source\t\tMax sidecar\t\tMin source\t\tMin sidecar\n")
				log.Printf("---------------------------------------------------------------------------------------------------------------------------------\n")
				log.Printf("%s\t\t%d\t\t%d\t\t\t%d\t\t\t%d\t\t\t%d\n",
					result.ID, result.MsgReceivedCount, result.FromSourceMaxDiff,
					result.FromSideCarMaxDiff, result.FromSourceMinDiff,
					result.FromSideCarMinDiff)
				pool.Put(result)

			}*/
		}
	}(latencyResult, &wg,cfg.Listener.Queue[0].Name)
	//}(&latencyBin, &wg)

	wg.Wait()

	log.Print("End Consumer")
}

func nakedAmqp(host string, queue string) {
	// Continuously read messages
	{

		// Create client
		client2, err := amqp.Dial(host)
		if err != nil {
			log.Fatal("Dialing AMQP server:", err)
		}
		defer client2.Close()

		// Open a session
		session2, err := client2.NewSession()
		if err != nil {
			log.Fatal("Creating AMQP session:", err)
		}

		// Create a receiver
		receiver, err := session2.NewReceiver(
			amqp.LinkSourceAddress(fmt.Sprintf("/%s", queue)),
			amqp.LinkCredit(50),
		)
		if err != nil {
			log.Fatal("Creating receiver link:", err)
		}
		ctx2 := context.Background()
		defer func() {
			ctx2, cancel2 := context.WithTimeout(ctx2, 5*time.Second)
			receiver.Close(ctx2)
			cancel2()
		}()

		for {
			// Receive next message
			msg, err := receiver.Receive(ctx2)
			if err != nil {
				log.Printf("Reading message from AMQP: %v", err)
			}

			// Accept message
			err = msg.Accept()
			if err != nil {
				log.Printf("Reading message from AMQP: %v", err)
			}
			data := types.Message{}
			err = json.Unmarshal(msg.GetData(), &data)
			if err != nil {
				log.Printf("Error marshalling event data %v", err)
			}

			atSourceDiff := time.Since(data.GetTime()).Milliseconds()
			//anything above 100ms is considered 100 ms
			if atSourceDiff >= int64(binSize) {
				atSourceDiff = int64(binSize)
			}
			/*r := MsgLatency{
				ID:       l.ID,
				MsgCount: int64(l.MsgReceivedCount),
				Latency:  atSourceDiff,
			}*/
			//latencyChan <- r
			latencyResults[atSourceDiff]++
			globalMsgReceivedCount++
		}

	}
}
