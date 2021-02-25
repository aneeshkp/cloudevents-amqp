package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	config2 "github.com/aneeshkp/cloudevents-amqp/pkg/config"
	eventconfig "github.com/aneeshkp/cloudevents-amqp/pkg/config"
	"github.com/aneeshkp/cloudevents-amqp/pkg/protocol"
	"github.com/aneeshkp/cloudevents-amqp/pkg/protocol/qdr"
	"github.com/aneeshkp/cloudevents-amqp/pkg/protocol/rest"
	"github.com/aneeshkp/cloudevents-amqp/pkg/report"
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

var (
	defaultSenderSocketPort   = 30001
	defaultListenerSocketPort = 30002
	defaultAPIPort            = 8081
	defaultHostPort           = 9091

	server          *rest.Server
	router          *qdr.Router
	qdrEventOutCh   chan protocol.DataEvent
	qdrEventInCh    chan protocol.DataEvent
	latencyCountCh  chan int64
	wg              sync.WaitGroup
	cfg             *config2.Config
	eventConsumeURL string
	//FOR PTP only
	statusCheckAddress string
)

func init() {
	qdrEventOutCh = make(chan protocol.DataEvent, 10)
	qdrEventInCh = make(chan protocol.DataEvent, 10)
}

func main() {
	var err error

	cfg, err = eventconfig.GetConfig()
	if err != nil {
		log.Printf("Could not load configuration file --config, loading default queue\n")
		//swap socket ports
		cfg = eventconfig.DefaultConfig(defaultHostPort, defaultAPIPort, defaultSenderSocketPort, defaultListenerSocketPort,
			os.Getenv("MY_CLUSTER_NAME"), os.Getenv("MY_NODE_NAME"), os.Getenv("MY_NAMESPACE"), false)
		cfg.EventHandler = types.SOCKET
	}
	//swap the port to solve conflict
	senderPort := cfg.Socket.Sender.Port
	cfg.Socket.Sender.Port = cfg.Socket.Listener.Port
	cfg.Socket.Listener.Port = senderPort

	//Consumer vDU URL event are read from QDR by sidecar and posted to vDU
	eventConsumeURL = fmt.Sprintf("http://%s:%d%s/event/alert", cfg.Host.HostName, cfg.Host.Port, "/api/vdu/v1")
	//SIDECAR URL: PTP events are published via post to its sidecar
	//eventPublishURL = fmt.Sprintf("http://%s:%d%s/event/create", sidCarAPIHostName, sideCarAPIPort, rest.SUBROUTINE)

	log.Printf("Connecting to host %s:%d", cfg.AMQP.HostName, cfg.AMQP.Port)
	//Sender sitting and waiting either to send or just create address or create address and send
	router = qdr.InitServer(cfg, qdrEventInCh, qdrEventOutCh)
	//qdrEventInCh   // qdr gets data from here via rest api to send data out or create adr address or to listen
	//router.DataOut = qdrEventOutCh // qdr writes out to this channel when message is received , acknowledged etc .
	// Initialize QDR router configurations
	router.QDRRouter(&wg)

	// QUE to listen for status
	if cfg.PublishStatus {
		statusCheckAddress = fmt.Sprintf("/%s/%s/%s", cfg.Cluster.Name, cfg.Cluster.Node, "status")
		qdrEventInCh <- protocol.DataEvent{
			Address:     statusCheckAddress,
			EventStatus: protocol.NEW,
			PubSubType:  protocol.STATUS,
		}
	}
	//rest api writes data to qdrEventInCh, which is consumed by QDR
	server = rest.InitServer(cfg, qdrEventInCh)

	//start http server
	wg.Add(1)
	go func() {
		defer wg.Done()
		server.Start()
	}()

	//health check sidecar rest api
	healthChk()
	//Latency report

	if cfg.EventHandler == types.SOCKET {
		SocketListener(&wg, cfg.Socket.Listener.Port, qdrEventInCh)
	} else if cfg.EventHandler == types.HTTP {
		//send count to this channel
		latencyCountCh = make(chan int64, 10)
		latencyReport := report.New(&wg, "default")
		latencyReport.Collect(latencyCountCh)
	}

	//listen to data coming out of qdrEventOutCh
	for { //nolint:gosimple
		select { //nolint:gosimple
		case d := <-qdrEventOutCh: // do something that is put out by QDR
			//Special handle need to redesign
			if d.PubSubType == protocol.STATUS {
				status := checkPTPStatus(&wg)
				resourceStatus := types.ResourceStatus{}
				err := json.Unmarshal(d.Data.Data(), &resourceStatus)
				if err != nil {
					log.Printf("Error marshalling event data when reading from QDR %v", err)
					resourceStatus.Status = fmt.Sprintf("error %v", err)
					_ = d.Data.SetData(cloudevents.ApplicationJSON, resourceStatus)
					//if it fails then we cant get return address
				} else {
					if status != "" {
						resourceStatus.Status = status
						_ = d.Data.SetData(cloudevents.ApplicationJSON, resourceStatus)
					} else {
						resourceStatus.Status = "could not get PTP status"
						_ = d.Data.SetData(cloudevents.ApplicationJSON, resourceStatus)
					}

					sender, err := qdr.NewSender(cfg.AMQP.HostName, cfg.AMQP.Port, resourceStatus.ReturnAddress)
					if err != nil {
						log.Printf("Failed to send: %s", resourceStatus.ReturnAddress)
						continue
					}
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					if result := sender.Client.Send(ctx, d.Data); cloudevents.IsUndelivered(result) {
						log.Printf("Failed to send: %v", result)
					} else if cloudevents.IsNACK(result) {
						log.Printf("Event not accepted: %v", result)
					}
					cancel()

				}

				continue
			}

			data := types.Subscription{}
			err := json.Unmarshal(d.Data.Data(), &data)
			if err != nil {
				log.Printf("Error marshalling event data when reading from QDR %v", err)
			} else {
				// find the endpoint you need to post
				if d.PubSubType == protocol.CONSUMER {
					if cfg.EventHandler == types.SOCKET {
						//now send events from QDR to CNF SOCKET port
						if d.EventStatus == protocol.NEW {
							sub := types.Subscription{}
							err := json.Unmarshal(d.Data.Data(), &sub)
							if err != nil {
								log.Printf("Failed to send events to CNF via socket %v", err)
							} else {
								if err := socketEvent(sub, cfg.Socket.Sender.Port); err != nil {
									log.Printf("Error sending to socket %v", err)
								}
							}
						}
					} else {
						processConsumer(d)
					}
				} else if d.PubSubType == protocol.PRODUCER {
					processProducer(d)
				}

			}

		}
	}
	//nolint:govet
	wg.Wait()
}

func processConsumer(event protocol.DataEvent) {
	// if its consumer then check subscription map TODO
	if event.EventStatus == protocol.NEW { // switch you endpoint url
		//fmt.Sprintf("http://%s:%d%s/event/alert", cnfAPIHostName, cnfAPIPort, "/api/vdu/v1")
		// check for subscription to fetch  the end url
		//TODO: This is required but takes lots of time , adds latency
		/*var endpointUri string
		for _,v:= range rest.SubscriptionStore{
			if v.ResourceQualifier.GetAddress()==d.Address{
				endpointUri=v.EndpointURI
				break
			}
		}*/
		if eventConsumeURL == "" {
			log.Printf("Could not find subscription for address %s", event.Address)
		} else {
			response, err := http.Post(eventConsumeURL, "application/json", bytes.NewBuffer(event.Data.Data()))
			if err != nil {
				log.Printf("publisher failed to post event to consumer %v for url %s", err, event.EndPointURI)
			} else {
				defer response.Body.Close()
				if response.StatusCode == http.StatusAccepted {
					bodyBytes, err := ioutil.ReadAll(response.Body)
					if err != nil {
						log.Printf("error reading response  after posting an event to consumer")
					} else {
						if cfg.EventHandler == types.HTTP { //for socket , collect report at sidecar level
							//body := []byte("{time:1}")
							dat := make(map[string]interface{})
							d := json.NewDecoder(bytes.NewBuffer(bodyBytes))
							d.UseNumber()
							if err := d.Decode(&dat); err != nil {
								log.Printf("Error parising latemcy %v", err)
								return
							}
							tags := dat["time"].(interface{})
							n := tags.(interface{}).(json.Number)
							i64, _ := strconv.ParseInt(string(n), 10, 64)
							latencyCountCh <- i64
						}

					}
					//log.Printf("Successfully post the event to the consumer vdu. %v",err)
				} else {
					log.Printf("publisher failed to post event to the consumer status %d", response.StatusCode)
				}
			}
		}
	} else {
		log.Printf("TODO:// handle  consumer data which is not new(what is that ?) %v", event.EventStatus)
	}
}
func processProducer(event protocol.DataEvent) {
	if event.EventStatus == protocol.SUCCEED {
		for _, v := range rest.PublisherStore {
			func(t types.Subscription) {
				if v.ResourceQualifier.GetAddress() == event.Address {
					// post it
					jsonValue, _ := json.Marshal(v)
					response, err := http.Post(v.EndpointURI, "application/json", bytes.NewBuffer(jsonValue))
					if err != nil {
						log.Printf("failed to post event ack to producer %v", err)
						return
					}
					defer response.Body.Close()
					if response.StatusCode != http.StatusAccepted {
						log.Printf("failed to send event  ack to CNF %d", response.StatusCode)

					}
				}
			}(v)
		}
	}
}

//Event will generate random events
func socketEvent(payload types.Subscription, port int) error {
	//log.Printf("coming to  send consumed data to CNF via SOCKET")
	Conn, _ := net.DialUDP("udp", nil, &net.UDPAddr{IP: []byte{127, 0, 0, 1}, Port: port, Zone: ""})
	defer Conn.Close()
	now := time.Now()
	nanos := now.UnixNano()
	// Note that there is no `UnixMillis`, so to get the
	// milliseconds since epoch you'll need to manually
	// divide from nanoseconds.
	millis := nanos / 1000000
	payload.EventTimestamp = millis

	b, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("Error: %s", err)
		return err
	}
	if _, err = Conn.Write(b); err != nil {
		log.Printf("Is there any error in udp %v", err)
		return err
	}
	//fmt.Printf("Sending %v messages\n", payload)

	return nil
}

func healthChk() {
	log.Printf("health check %s ", fmt.Sprintf("http://%s:%d%s/health", cfg.API.HostName, cfg.API.Port, rest.SUBROUTINE))
	for {
		response, err := http.Get(fmt.Sprintf("http://%s:%d%s/health", cfg.API.HostName, cfg.API.Port, rest.SUBROUTINE))
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		if response != nil && response.StatusCode == http.StatusOK {
			response.Body.Close()
			return
		}
		response.Body.Close()

		time.Sleep(2 * time.Second)
	}
}

//SocketListener creates a socket listener
func SocketListener(wg *sync.WaitGroup, udpListenerPort int, dataOut chan<- protocol.DataEvent) {
	wg.Add(1)
	go func(wg *sync.WaitGroup, udpListenerPort int) {
		defer wg.Done()
		ServerConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: []byte{0, 0, 0, 0}, Port: udpListenerPort, Zone: ""})
		if err != nil {
			log.Fatalf("Error setting up socket %v", err)
		}
		defer ServerConn.Close()
		buffer := make([]byte, 1024)
		for {
			n, _, err := ServerConn.ReadFromUDP(buffer)
			if err != nil {
				log.Println(err)
				continue
			}
			event := cloudevents.NewEvent()
			sub := types.Subscription{}
			err = json.Unmarshal(buffer[:n], &sub)
			if err != nil {
				log.Println(err)
				continue
			}
			event.SetID(uuid.New().String())
			event.SetSource("https://github.com/aneeshkp/cloud-events/producer")
			event.SetTime(time.Now())
			event.SetType("com.cloudevents.poc.event.sent")
			event.SetSpecVersion(cloudevents.VersionV1)
			err = event.SetData(cloudevents.ApplicationJSON, buffer[:n])
			if err != nil {
				log.Println(err)
				continue
			}
			d := protocol.DataEvent{
				Address:     sub.ResourceQualifier.GetAddress(),
				Data:        event,
				PubSubType:  protocol.PRODUCER,
				EventStatus: 0,
				EndPointURI: sub.EndpointURI,
			}
			fmt.Printf("data sent %v", d)
			dataOut <- d //send to QDR (Now only events are sent via socket , rest happens via http)
		}
	}(wg, udpListenerPort)
}
func checkPTPStatus(wg *sync.WaitGroup) string {
	return "PTP SOCKET IS DOING AWESOME"
}

/*func checkPTPStatus(wg *sync.WaitGroup) string {

	fn:=func(wd *sync.WaitGroup, c net.Conn, r chan<-string)  {
		defer wg.Done()
		received := make([]byte, 0)
		defer close(r)
		for {
			buf := make([]byte, 512)
			count, err := c.Read(buf)

			if err != nil {
				if err != io.EOF {
					log.Printf("Error on read: %s", err)
					close(r)
					return
				}

			}else{
				fmt.Printf("ack to me %s",string(received))
				r<-string(buf[:count])
			}
		}
	}

	s:=make(chan string)
	c, err := net.Dial("unix", "/tmp/ptp.sock")
	if err != nil {
		return fmt.Sprintf("Failed to dial: %s", err)
	}
	time.Sleep(2*time.Second)
	wg.Add(1)
	go fn(wg,c,s)
	defer c.Close()
	_, err = c.Write([]byte("Hi"))

	if err != nil {
		return fmt.Sprintf("Write error: %s", err)
	}
	sa:=<-s
	close(s)
	return sa

}*/
