package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	amqpconfig "github.com/aneeshkp/cloudevents-amqp/pkg/config/amqp"
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
	udpSenderPort   = 30001
	udpListenerPort = 30002

	server            *rest.Server
	router            *qdr.Router
	qdrEventOutCh     chan protocol.DataEvent
	qdrEventInCh      chan protocol.DataEvent
	latencyCountCh    chan int64
	wg                sync.WaitGroup
	pubFilePath       = "pub.json"
	subFilePath       = "sub.json"
	sidCarAPIHostName = "localhost"
	sideCarAPIPort    = 8080
	cnfAPIHostName    = "localhost"
	cnfAPIPort        = 9090
	cfg               *amqpconfig.Config
	eventConsumeURL   string
	eventHandler      = types.HTTP
)

func init() {
	qdrEventOutCh = make(chan protocol.DataEvent, 10)
	qdrEventInCh = make(chan protocol.DataEvent, 10)
}


func main() {

	cfg, _ = config()
	//Consumer vDU URL event are read from QDR by sidecar and posted to vDU
	eventConsumeURL = fmt.Sprintf("http://%s:%d%s/event/alert", cnfAPIHostName, cnfAPIPort, "/api/cnf/v1")
	//SIDECAR URL: PTP events are published via post to its sidecar
	//eventPublishURL = fmt.Sprintf("http://%s:%d%s/event/create", sidCarAPIHostName, sideCarAPIPort, rest.SUBROUTINE)

	log.Printf("Connecting to host %s:%d", cfg.HostName, cfg.Port)
	//Sender sitting and waiting either to send or just create address or create address and send
	router = qdr.InitServer(cfg.HostName, cfg.Port, qdrEventInCh, qdrEventOutCh)
	//qdrEventInCh   // qdr gets data from here via rest api to send data out or create adr address or to listen
	//router.DataOut = qdrEventOutCh // qdr writes out to this channel when message is received , acknowledged etc .
	// Initialize QDR router configurations
	router.QDRRouter(&wg)

	//rest api writes data to qdrEventInCh, which is consumed by QDR
	server = rest.InitServer(sidCarAPIHostName, sideCarAPIPort, pubFilePath, subFilePath, qdrEventInCh)

	//start http server
	wg.Add(1)
	go func() {
		defer wg.Done()
		server.Start()
	}()

	//health check sidecar rest api
	healthChk()
	//Latency report

	if eventHandler == types.SOCKET {
		SocketListener(&wg, udpListenerPort, qdrEventInCh)
	} else if eventHandler == types.HTTP {
		//send count to this channel
		latencyCountCh = make(chan int64, 10)
		latencyReport := report.New(&wg, "default")
		latencyReport.Collect(latencyCountCh)
	}

	//listen to data coming out of qdrEventOutCh
	for { //nolint:gosimple
		select { //nolint:gosimple
		case d := <-qdrEventOutCh: // do something that is put out by QDR
			data := types.Subscription{}
			err := json.Unmarshal(d.Data.Data(), &data)
			if err != nil {
				log.Printf("Error marshalling event data when reading from QDR %v", err)
			} else {
				// find the endpoint you need to post
				if d.PubSubType == protocol.CONSUMER {
					if eventHandler == types.SOCKET {
						//now send events from QDR to CNF SOCKET port
						if d.EventStatus == protocol.NEW {
							sub := types.Subscription{}
							err := json.Unmarshal(d.Data.Data(), &sub)
							if err != nil {
								log.Printf("Failed to send events to CNF via socket %v", err)
							} else {
								if err := socketEvent(sub); err != nil {
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
		//fmt.Sprintf("http://%s:%d%s/event/alert", cnfAPIHostName, cnfAPIPort, "/api/cnf/v1")
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

						if eventHandler == types.HTTP { //for socket , collect report at sidecar level
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
					//log.Printf("Successfully post the event to the consumer cnf. %v",err)
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
func socketEvent(payload types.Subscription) error {
	//log.Printf("coming to  send consumed data to CNF via SOCKET")
	Conn, _ := net.DialUDP("udp", nil, &net.UDPAddr{IP: []byte{127, 0, 0, 1}, Port: udpSenderPort, Zone: ""})
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
	for {
		response, err := http.Get(fmt.Sprintf("http://%s:%d%s/health", sidCarAPIHostName, sideCarAPIPort, rest.SUBROUTINE))
		if err != nil {
			log.Printf("Error connecting to rest api %v", err)
			continue
		}
		if response != nil && response.StatusCode == http.StatusOK {
			response.Body.Close()
			return
		}
		response.Body.Close()
		log.Printf("waiting for sidecar rest service to start\n")
		time.Sleep(2 * time.Second)
	}

}

func config() (*amqpconfig.Config, error) {
	envSidCarAPIHostName := os.Getenv("SC_API_HOST_NAME")
	if envSidCarAPIHostName != "" {
		sidCarAPIHostName = envSidCarAPIHostName
	}
	envSideCarAPIPort := os.Getenv("SC_API_PORT")
	if envSideCarAPIPort != "" {
		sideCarAPIPort, _ = strconv.Atoi(envSideCarAPIPort)
	}
	envCnfAPIHostName := os.Getenv("API_HOST_NAME")
	if envCnfAPIHostName != "" {
		cnfAPIHostName = envCnfAPIHostName
	}
	envCnfAPIPort := os.Getenv("API_PORT")
	if envCnfAPIPort != "" {
		cnfAPIPort, _ = strconv.Atoi(envCnfAPIPort)
	}
	envEventHandler := os.Getenv("EVENT_HANDLER")
	if envEventHandler != "" {
		eventHandler = types.EventHandler(envEventHandler)
	}
	cfg, err := amqpconfig.GetConfig()
	if err != nil {
		log.Printf("Could not load configuration file --config, loading default queue\n")
		cfg = &amqpconfig.Config{
			HostName: "amqp://localhost",
			Port:     5672,
		}
	}
	return cfg, err
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
			dataOut <- d //send to QDR (Now only events are sent via socket , rest happens via http)
		}
	}(wg, udpListenerPort)
}
