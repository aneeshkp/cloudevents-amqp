package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Azure/go-amqp"
	"github.com/aneeshkp/cloudevents-amqp/types"
	"log"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	amqp_config "github.com/aneeshkp/cloudevents-amqp/pkg/config/amqp"
	listener_type "github.com/aneeshkp/cloudevents-amqp/pkg/types"
	amqp1 "github.com/cloudevents/sdk-go/protocol/amqp/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
)

const (
	defaultMsgCount = 10
)

var (
	msgReceivedCount     uint64 = 0
	msgCurrentBatchCount uint64 = 0
	maxDiff              int64  = 0
	currentBatchMaxDiff  int64  = 0
	cfg                  *amqp_config.Config
	wg                   sync.WaitGroup
)

// Parse AMQP_URL env variable. Return server URL, AMQP node (from path) and SASLPlain
// option if user/pass are present.
func amqpConfig() (server, node string, opts []amqp1.Option) {
	env := os.Getenv("AMQP_URL")
	if env == "" {
		env = "/test"
	}
	u, err := url.Parse(env)
	if err != nil {
		log.Fatal(err)
	}
	if u.User != nil {
		user := u.User.Username()
		pass, _ := u.User.Password()
		opts = append(opts, amqp1.WithConnOpt(amqp.ConnSASLPlain(user, pass)))
	}
	opts = append(opts, amqp1.WithReceiverLinkOption(amqp.LinkCredit(50)))
	//opts = append(opts, amqp1.WithReceiverLinkOption(amqp.LinkReceiverSettle(amqp.ModeFirst)))
	//opts = append(opts, amqp1.WithSenderLinkOption(amqp.LinkSenderSettle(amqp.ModeSettled)))
	return env, strings.TrimPrefix(u.Path, "/"), opts
}

func main() {
	host, _, opts := amqpConfig()
	var listeners []listener_type.AMQPProtocol

	var p *amqp1.Protocol
	var err error
	log.Printf("Connecting to host %s", host)

	cfg, err := amqp_config.GetConfig()
	if err != nil {
		log.Print("Could not load configuration file --config, loading default queue\n")
		cfg = &amqp_config.Config{
			Listener: amqp_config.Listener{
				Count: defaultMsgCount,
				Queue: []amqp_config.Queue{
					{
						Name:  "test",
						Count: 2,
					},
				},
			},
		}
	}
	for _, q := range cfg.Listener.Queue {
		for i:=1; i<=q.Count; i++ {
			l := listener_type.AMQPProtocol{}
			l.Queue = q.Name
			l.ID=fmt.Sprintf("%s-#%d",l.Queue,i)
			for {
				p, err = amqp1.NewProtocol2(host, "", l.Queue, []amqp.ConnOption{}, []amqp.SessionOption{}, opts...)
				if err != nil {
					log.Printf("Failed to create amqp protocol (trying in 5 secs): %v", err)
					time.Sleep(5 * time.Second)
				} else {
					log.Printf("Connection established for consumer %s\n",l.ID)
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
			listeners = append(listeners, l)
		}
	}

	for _, l := range listeners {

		wg.Add(1)
		go func(l *listener_type.AMQPProtocol) {
			fmt.Printf("listenining to queue %s by %s\n",l.Queue,l.ID)
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
				if (int(l.MsgReceivedCount) % cfg.Listener.Count)== 0 {
					fmt.Printf("\n CE-AMQP: Total message recived for queue %s = %d, maxDiff = %d\n", l.ID,l.MsgReceivedCount,  l.MaxDiff)
					//fmt.Printf("CE-AMQP: Total current batch message recived for queue %s = %d, maxDiff = %d\n", msgCurrentBatchCount, l.Queue, currentBatchMaxDiff)
				}

			})
			if err != nil {
				log.Printf("AMQP receiver error: %v", err)
			}

		}(&l)

	}

	wg.Wait()




	log.Print("End Consumer")
}
