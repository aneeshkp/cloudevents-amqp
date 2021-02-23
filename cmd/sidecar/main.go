package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	amqpconfig "github.com/aneeshkp/cloudevents-amqp/pkg/config/amqp"
	"github.com/aneeshkp/cloudevents-amqp/pkg/protocol"
	"github.com/aneeshkp/cloudevents-amqp/pkg/protocol/qdr"
	"github.com/aneeshkp/cloudevents-amqp/pkg/protocol/rest"
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

const (
	maxBinSize = 1002
)

var (

	udpSenderPort = 20001
	udpListenerPort = 20002
	server            *rest.Server
	router            *qdr.Router
	qdrEventOutCh     chan protocol.DataEvent
	qdrEventInCh      chan protocol.DataEvent
	wg                sync.WaitGroup
	cnfType           = "PUBLISHER"
	pubFilePath       = "pub.json"
	subFilePath       = "sub.json"
	sidCarApiHostName = "localhost"
	sideCarApiPort    = 8080
	cnfApiHostName    = "localhost"
	cnfApiPort        = 9090
	cfg               *amqpconfig.Config

	latencyResult  map[string]*latency
	latencyResults []int
	//channelBufferSize      = bufferSize
	binSize                = maxBinSize
	globalMsgReceivedCount int64
	eventHandler=types.HTTP
)

type LCount struct {
	Time int64 `json: "time,omitempty,string"`
}
type latency struct {
	ID       string
	MsgCount int64
	Latency  [maxBinSize]int64
}

func init() {
	qdrEventOutCh = make(chan protocol.DataEvent, 10)
	qdrEventInCh = make(chan protocol.DataEvent, 10)
}

// DataEvent ...

func main() {
	envSidCarApiHostName := os.Getenv("SC_API_HOST_NAME")
	if envSidCarApiHostName != "" {
		sidCarApiHostName = envSidCarApiHostName
	}
	envSideCarApiPort := os.Getenv("SC_API_PORT")
	if envSideCarApiPort != "" {
		sideCarApiPort, _ = strconv.Atoi(envSideCarApiPort)
	}
	envCnfApiHostName := os.Getenv("API_HOST_NAME")
	if envCnfApiHostName != "" {
		cnfApiHostName = envCnfApiHostName
	}
	envCnfApiPort := os.Getenv("API_PORT")
	if envCnfApiPort != "" {
		cnfApiPort, _ = strconv.Atoi(envCnfApiPort)
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
	log.Printf("Connecting to host %s:%d", cfg.HostName, cfg.Port)
	//Sender sitting and waiting either to send or just create address or create address and send
	router = qdr.InitServer(cfg.HostName, cfg.Port)
	router.DataIn = qdrEventInCh   // qdr gets data from here via rest api to send data out or create adr address or to listen
	router.DataOut = qdrEventOutCh // qdr writes out to this channel when message is received , acknowledged etc . Need to implment this
	// Initialize
	wg.Add(1)
	go router.QDRRouter(&wg)

	server = rest.InitServer(sidCarApiHostName, sideCarApiPort, pubFilePath, subFilePath)
	server.DataOut = qdrEventInCh // Here api server write to this change

	//start http server
	wg.Add(1)
	go func() {
		defer wg.Done()
		server.Start()
	}()

	//health check sidecar rest api
	for {
		response, err := http.Get(fmt.Sprintf("http://%s:%d%s/health", sidCarApiHostName, sideCarApiPort, rest.SUBROUTINE))
		if err != nil {
			log.Printf("Error connecting to rest api %v", err)
			continue
		}
		if response != nil && response.StatusCode == http.StatusOK {
			response.Body.Close()
			break
		}
		response.Body.Close()
		log.Printf("waiting for sidecar rest service to start\n")
		time.Sleep(2 * time.Second)

	}
	if eventHandler==types.UDP{
		wg.Add(1)
		go udpSetup(&wg,"localhost",udpListenerPort)
	}
	latencyResult = make(map[string]*latency, 1)
	latencyResults = make([]int, binSize)
	if eventHandler==types.HTTP {
		wg.Add(1)

		go func(latencyResult map[string]*latency, wg *sync.WaitGroup, q string) {
			defer wg.Done()
			uptimeTicker := time.NewTicker(5 * time.Second)

			for { //nolint:gosimple
				select {
				case <-uptimeTicker.C:
					fmt.Printf("|%-15s|%15s|%15s|%15s|%15s|", "ID", "Total Msg", "Latency(ms)", "Msg", "Histogram(%)")
					fmt.Println()

					var j int64
					for i := 0; i < binSize; i++ {
						latency := latencyResults[i]
						li := float64(globalMsgReceivedCount)
						if latency > 0 {
							fmt.Printf("|%-15s|%15d|%15d|%15d|", q, globalMsgReceivedCount, i, latency)
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
		}(latencyResult, &wg, "default")
	}
	// loop amqp remote-listener
	for { //nolint:gosimple
		func() {
			select {
			case d := <-qdrEventOutCh: // do something that is put out by QDR
				//log.Printf("this is data coming from %s and %s", d.PubSubType,d.EventStatus)
				data := types.Subscription{}
				err := json.Unmarshal(d.Data.Data(), &data)
				if err != nil {
					log.Printf("Error marshalling event data when reading from QDR %v", err)
				} else {
					// find the endpoint you need to post
					if d.PubSubType == protocol.CONSUMER {
						if eventHandler == types.UDP {
							//now send events from QDR to CNF UDP port
							if d.EventStatus == protocol.NEW {
								sub := types.Subscription{}
								err := json.Unmarshal(d.Data.Data(), &sub)
								if err != nil {
									log.Printf("Failed to send events to CNF via socker %v", err)
								} else {
									udpEvent(sub)
								}
							}
						} else {
							// if its consumer then check subscription map
							if d.EventStatus == protocol.NEW {
								response, err := http.Post(fmt.Sprintf("http://%s:%d%s/event/alert", cnfApiHostName, cnfApiPort, "/api/cnf/v1"), "application/json", bytes.NewBuffer(d.Data.Data()))
								if err != nil {
									log.Printf("failed to post event to consumer %v for url %s", err, d.EndPointURi)
								} else {
									defer response.Body.Close()
									if response.StatusCode == http.StatusAccepted {
										bodyBytes, err := ioutil.ReadAll(response.Body)
										if err != nil {
											log.Printf("error reading response  after posting an event to consumer")
										} else {
											latency := LCount{}
											err = json.Unmarshal(bodyBytes, &latency)
											if err != nil {
												log.Printf("Error reading latency %v for %s", err, string(bodyBytes))
											} else {
												//anything above 100ms is considered 100 ms
												if eventHandler == types.HTTP {
													if latency.Time >= int64(binSize) {
														latency.Time = int64(binSize - 1)
													}
													latencyResults[latency.Time]++
													globalMsgReceivedCount++
												}
											}
										}
										//log.Printf("Successfully post the event to the consumer cnf. %v",err)
									} else {
										log.Printf("failed to post event to the consumer cnf %d", response.StatusCode)
									}
								}
							} else {
								log.Printf("TODO:// handle  consumer data which is not new(what is that ?) %s", d.EventStatus)
							}
						}
					} else if d.PubSubType == protocol.PRODUCER {
						if d.EventStatus == protocol.SUCCEED {
							for _, v := range rest.PublisherStore {
								func(t types.Subscription){
									if v.ResourceQualifier.GetAddress() == d.Address {
										// post it
										jsonValue, _ := json.Marshal(v)
										response, err := http.Post(v.EndpointUri, "application/json", bytes.NewBuffer(jsonValue))
										if err != nil {
											log.Printf("failed to post event ack to producer %v", err)
											return
										} else {
											defer response.Body.Close()
											if response.StatusCode == http.StatusAccepted {
												//log.Printf("Event was recieved by CNF . %v",err)
											} else {
												log.Printf("failed to send event  ack to CNF %d", response.StatusCode)
											}
										}
									}
								}(v)
							}
						}

					}

				}

			}
		}()
	}
	//nolint:govet

	wg.Wait()

}

func udpSetup(wg *sync.WaitGroup,hostname string, udpListenerPort int) {
	defer wg.Done()
	ServerConn, _ := net.ListenUDP("udp", &net.UDPAddr{IP:[]byte{0,0,0,0},Port:udpListenerPort,Zone:""})
	defer ServerConn.Close()
	buffer := make([]byte, 1024)
	for {
		n, _, err  := ServerConn.ReadFromUDP(buffer)
		if err!=nil{
			continue
		}
		event := cloudevents.NewEvent()
		sub :=types.Subscription{}
		err=json.Unmarshal(buffer[:n],&sub)
		event.SetID(uuid.New().String())
		event.SetSource("https://github.com/aneeshkp/cloud-events/producer")
		event.SetTime(time.Now())
		event.SetType("com.cloudevents.poc.event.sent")
		event.SetData(cloudevents.ApplicationJSON, buffer[:n])
		d:=protocol.DataEvent{
			Address:     sub.ResourceQualifier.GetAddress(),
			Data:        event,
			PubSubType: protocol.PRODUCER ,
			EventStatus: 0,
			EndPointURi: sub.EndpointUri,
		}
		qdrEventInCh <- d //send to QDR (Now only events are sent via UDP , rest happens via http)

	}
}
func handleUDPConnection(conn *net.UDPConn) {
	buffer := make([]byte, 1500)
	n, _, err := conn.ReadFromUDP(buffer)
	if err != nil {
		//log.Print(err)
		return
	}
	event := cloudevents.NewEvent()

	sub :=types.Subscription{}
	err=json.Unmarshal(buffer[:n],&sub)
	if err!=nil{
		log.Printf("Error marshalling events")
		return
	}
	event.SetID(uuid.New().String())
	event.SetSource("https://github.com/aneeshkp/cloud-events/producer")
	event.SetTime(time.Now())
	event.SetType("com.cloudevents.poc.event.sent")
	event.SetData(cloudevents.ApplicationJSON, buffer[:n])
	d:=protocol.DataEvent{
		Address:     sub.ResourceQualifier.GetAddress(),
		Data:        event,
		PubSubType: protocol.PRODUCER ,
		EventStatus: 0,
		EndPointURi: sub.EndpointUri,
	}
	log.Printf("message recieved %v\n",event)
	qdrEventInCh <- d //send to QDR (Now only events are sent via UDP , rest happens via http)
	//fmt.Println("UDP client : ", addr)
	//fmt.Println("Received from UDP client :  ", string(buffer[:n]))
}


//Event will generate random events
func udpEvent(payload types.Subscription) error {
	//log.Printf("coming to  send consumed data to CNF via UDP")
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
		log.Printf("Is there any error in udp %v",err)
		return err
	}
	//fmt.Printf("Sending %v messages\n", payload)

	return nil
}
