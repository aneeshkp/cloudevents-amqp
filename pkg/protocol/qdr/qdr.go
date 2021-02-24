package qdr

import (
	"context"
	"fmt"
	"github.com/Azure/go-amqp"
	"github.com/aneeshkp/cloudevents-amqp/pkg/protocol"
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
	amqp1 "github.com/cloudevents/sdk-go/protocol/amqp/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"log"
	"sync"
	"time"
)

/*var (
	_ protocol.Protocol = (*Router)(nil)
)*/

//Router defines QDR router object
type Router struct {
	Listeners map[string]*types.AMQPProtocol
	Senders   map[string]*types.AMQPProtocol
	DataIn    <-chan protocol.DataEvent
	cfg       protocol.Config
	DataOut   chan<- protocol.DataEvent
}

//InitServer initialize QDR configurations
func InitServer(hostname string, port int, DataIn <-chan protocol.DataEvent, DataOut chan<- protocol.DataEvent) *Router {
	server := Router{
		Listeners: map[string]*types.AMQPProtocol{},
		Senders:   map[string]*types.AMQPProtocol{},
		DataIn:    DataIn,
		cfg: protocol.Config{
			HostName: hostname,
			Port:     port,
		},
		DataOut: DataOut,
	}
	return &server
}

//NewSender creates new QDR sender
func (q *Router) NewSender(address string) error {
	var opts []amqp1.Option
	p, err := amqp1.NewProtocol2(fmt.Sprintf("%s:%d", q.cfg.HostName, q.cfg.Port), address, "", []amqp.ConnOption{}, []amqp.SessionOption{}, opts...)
	if err != nil {
		log.Printf("Failed to create amqp protocol: %v", err)
		return err
	}
	log.Printf("(New) Sender Connection established %s\n", address)
	//fn := reflect.ValueOf(fn)
	l := types.AMQPProtocol{}
	c, err := cloudevents.NewClient(p)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	l.Protocol = p
	l.Client = c
	q.Senders[address] = &l

	return nil
}

//NewReceiver creates new QDR receiver
func (q *Router) NewReceiver(address string) error {
	var opts []amqp1.Option
	opts = append(opts, amqp1.WithReceiverLinkOption(amqp.LinkCredit(50)))
	p, err := amqp1.NewProtocol2(fmt.Sprintf("%s:%d", q.cfg.HostName, q.cfg.Port), "", address, []amqp.ConnOption{}, []amqp.SessionOption{}, opts...)
	if err != nil {
		log.Printf("Failed to create amqp protocol for a Receiver: %v", err)
		return err
	}
	log.Printf("(New Receiver) Connection established %s\n", address)
	//fn := reflect.ValueOf(fn)
	l := types.AMQPProtocol{}
	parent, cancelParent := context.WithCancel(context.Background())
	l.CancelFn = cancelParent
	l.ParentContext = parent
	c, err := cloudevents.NewClient(p)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	l.Protocol = p
	l.Client = c
	q.Listeners[address] = &l

	return nil
}

//Receive is a QDR receiver listening to a address specified
func (q *Router) Receive(wg *sync.WaitGroup, address string, fn func(e cloudevents.Event)) {
	var err error
	defer wg.Done()
	if val, ok := q.Listeners[address]; ok {
		err = val.Client.StartReceiver(context.Background(), fn)
		if err != nil {
			log.Printf("AMQP receiver error: %v", err)
		}
	}
}

//ReceiveAll creates receiver to all address and receives events for all address
func (q *Router) ReceiveAll(wg *sync.WaitGroup, fn func(e cloudevents.Event)) {
	defer wg.Done()
	var err error
	for _, l := range q.Listeners {
		wg.Add(1)
		go func(l *types.AMQPProtocol, wg *sync.WaitGroup) {
			fmt.Printf("listenining to queue %s by %s\n", l.Queue, l.ID)
			defer wg.Done()
			err = l.Client.StartReceiver(context.Background(), fn)
			if err != nil {
				log.Printf("AMQP receiver error: %v", err)
			}
		}(l, wg)
	}

}

//QDRRouter is qpid router object configured to create publishers and  consumers
func (q *Router) QDRRouter(wg *sync.WaitGroup) {
	wg.Add(1)
	go func(q *Router, wg *sync.WaitGroup) {
		defer wg.Done()
		for { //nolint:gosimple
			select {
			case d := <-q.DataIn:
				if d.PubSubType == protocol.CONSUMER {
					log.Printf("reading from data %s", d.Address)
					if d.Address != "" && d.Data.Data() == nil { //create new address protocol
						if _, ok := q.Listeners[d.Address]; !ok {
							log.Printf("(1)Listener not found for the following address %s , creating listener", d.Address)
							err := q.NewReceiver(d.Address)
							if err != nil {
								log.Printf("Error creating Receiver %v", err)
							} else {
								wg.Add(1)
								go q.Receive(wg, d.Address, func(e cloudevents.Event) {
									//log.Printf("Received event  %s", string(e.Data()))
									// Not the clean way of doing , revisit
									q.DataOut <- protocol.DataEvent{
										Address:     d.Address,
										Data:        e,
										EventStatus: protocol.NEW,
										PubSubType:  protocol.CONSUMER,
									}
								})
							}
						}
					}
				} else if d.PubSubType == protocol.PRODUCER {
					//log.Printf("reading from data %s", d.Address)
					if d.Address != "" && d.Data.Data() == nil { //create new address protocol
						if _, ok := q.Senders[d.Address]; !ok {
							log.Printf("(1)Sender not found for the following address %s , creating sender", d.Address)
							err := q.NewSender(d.Address)
							if err != nil {
								log.Printf("Error creating sender %v", err)
							}
						}
					} else if d.Data.Data() != nil && d.Address == "" { //send it to all
						log.Printf("data %v", d.Data)
						log.Printf("dd %#v", q.Senders["test"])
						q.SendToAll(wg, d.Data)
					} else if d.Data.Data() != nil && d.Address != "" { //send to specific address
						if _, ok := q.Senders[d.Address]; !ok {
							log.Printf("(2)Sender not found for the following address %s, creating sender", d.Address)
							err := q.NewSender(d.Address)
							if err != nil {
								log.Printf("Error creating sender %v", err)
							}
						}
						q.SendTo(wg, d.Address, d.Data)
					}
				}
			}
		}
	}(q, wg)
}

//SendTo sends events to the address specified
func (q *Router) SendTo(wg *sync.WaitGroup, address string, event cloudevents.Event) {
	if val, ok := q.Senders[address]; ok {
		wg.Add(1) //for each sender you send a message since its
		go func(s *types.AMQPProtocol, e cloudevents.Event, wg *sync.WaitGroup) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(10)*time.Second)
			defer cancel()
			if result := s.Client.Send(ctx, event); cloudevents.IsUndelivered(result) {
				log.Printf("Failed to send: %v", result)
				q.DataOut <- protocol.DataEvent{
					Address:     address,
					Data:        e,
					EventStatus: protocol.FAILED,
					PubSubType:  protocol.PRODUCER,
				}
			} else if cloudevents.IsNACK(result) {
				log.Printf("Event not accepted: %v", result)
				q.DataOut <- protocol.DataEvent{
					Address:     address,
					Data:        e,
					EventStatus: protocol.SUCCEED,
					PubSubType:  protocol.PRODUCER,
				}
			}
		}(val, event, wg)
	}
}

// SendToAll ... sends events to all registered qdr address
func (q *Router) SendToAll(wg *sync.WaitGroup, event cloudevents.Event) {
	for k, s := range q.Senders {
		wg.Add(1) //for each sender you send a message since its
		go func(s *types.AMQPProtocol, address string, e cloudevents.Event, wg *sync.WaitGroup) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(1)*time.Second)
			defer cancel()
			if result := s.Client.Send(ctx, event); cloudevents.IsUndelivered(result) {
				log.Printf("Failed to send: %v", result)
				q.DataOut <- protocol.DataEvent{
					Address:     address,
					Data:        e,
					EventStatus: protocol.FAILED,
					PubSubType:  protocol.PRODUCER,
				} // Not the clean way of doing , revisit
			} else if cloudevents.IsNACK(result) {
				log.Printf("Event not accepted: %v", result)
				q.DataOut <- protocol.DataEvent{
					Address:     address,
					Data:        e,
					EventStatus: protocol.SUCCEED,
					PubSubType:  protocol.PRODUCER,
				} // Not the clean way of doing , revisit
			}
		}(s, k, event, wg)
	}
}
