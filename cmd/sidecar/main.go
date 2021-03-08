package main

// export PN_TRACE_FRM=1

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
	"github.com/aneeshkp/cloudevents-amqp/pkg/socket"
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
	"github.com/aneeshkp/cloudevents-amqp/pkg/types/status"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"
)

var (
	defaultSenderSocketPort   = 20001
	defaultListenerSocketPort = 20002
	defaultAPIPort            = 8080
	defaultHostPort           = 9090

	wg  sync.WaitGroup
	cfg *config2.Config
	//FOR ptp only
	statusCheckAddress string
	server             *rest.Server
	socketSenderCon    *net.UDPConn
)

func main() {
	var err error

	var router *qdr.Router
	var qdrEventOutCh chan protocol.DataEvent
	var qdrEventInCh chan protocol.DataEvent
	qdrEventOutCh = make(chan protocol.DataEvent, 100)
	qdrEventInCh = make(chan protocol.DataEvent, 100)

	cfg, err = eventconfig.GetConfig()
	if err != nil {
		log.Printf("Could not load configuration file --config, loading default queue\n")
		cfg = eventconfig.DefaultConfig(defaultHostPort, defaultAPIPort, defaultSenderSocketPort, defaultListenerSocketPort,
			os.Getenv("MY_CLUSTER_NAME"), os.Getenv("MY_NODE_NAME"), os.Getenv("MY_NAMESPACE"))
		//switching between socket and http
		cfg.HostPathPrefix = "/api/ptp/v1"
		cfg.APIPathPrefix = "/api/ocloudnotifications/v1"
		cfg.StatusResource.Status.PublishStatus = true
		cfg.StatusResource.Status.EnableStatusCheck = false
	}

	log.Printf("Framework type :%s\n", cfg.EventHandler)
	//swap the port to solve conflict, since side car and main containers are sending and listening to ports
	senderPort := cfg.Socket.Sender.Port
	cfg.Socket.Sender.Port = cfg.Socket.Listener.Port
	cfg.Socket.Listener.Port = senderPort

	//watchStoreUpdates(&wg)

	log.Printf("Connecting to qdr host %s:%d", cfg.AMQP.HostName, cfg.AMQP.Port)
	//Sender sitting and waiting either to send or just create address or create address and send
	router = qdr.InitServer(cfg, qdrEventInCh, qdrEventOutCh)
	//qdrEventInCh   // qdr gets data from here via rest api to send data out or create adr address or to listen
	//router.DataOut = qdrEventOutCh // qdr writes out to this channel when message is received , acknowledged etc .
	// Initialize QDR router configurations
	router.QDRRouter(&wg)

	//Start web services rest api writes data to qdrEventInCh, which is consumed by QDR
	server = rest.InitServer(cfg, qdrEventInCh)
	server = rest.InitServer(cfg, qdrEventInCh)
	statusListenerQueue := status.NewStatusListenerChannel(&wg)
	server.StatusListenerQueue = statusListenerQueue
	//start http server
	wg.Add(1)
	go func() {
		defer wg.Done()
		server.Start()
	}()

	//Special: Create a QDR Listener for listening to incoming status request
	if cfg.StatusResource.Status.PublishStatus {
		for _, name := range cfg.StatusResource.Name {
			statusPublishAddress := fmt.Sprintf("/%s/%s/%s", cfg.Cluster.Name, cfg.Cluster.Node, name)
			qdrEventInCh <- protocol.DataEvent{
				Address:     statusPublishAddress,
				EventStatus: protocol.NEW,
				PubSubType:  protocol.STATUS,
			}
		}
		server.StatusSenders = make(map[string]*types.AMQPProtocol)
		statusReturnAddress := fmt.Sprintf("/%s/%s/%s", cfg.Cluster.Name, cfg.Cluster.Node, "status/ptp/CurrentStatus")
		sender, _ := qdr.NewSender(cfg.AMQP.HostName, cfg.AMQP.Port, statusReturnAddress)
		server.StatusSenders[statusCheckAddress] = sender
	}
	if cfg.StatusResource.Status.EnableStatusCheck {
		server.StatusSenders = make(map[string]*types.AMQPProtocol)
		for _, name := range cfg.StatusResource.Name {
			statusCheckAddress := fmt.Sprintf("/%s/%s/%s", cfg.Cluster.Name, cfg.Cluster.Node, name)
			sender, _ := qdr.NewSender(cfg.AMQP.HostName, cfg.AMQP.Port, statusCheckAddress)
			server.StatusSenders[statusCheckAddress] = sender
		}
		//listener to get the status
		statusReturnAddress := fmt.Sprintf("/%s/%s/%s", cfg.Cluster.Name, cfg.Cluster.Node, "status/ptp/CurrentStatus")
		qdrEventInCh <- protocol.DataEvent{
			Address:     statusReturnAddress,
			EventStatus: protocol.SUCCEED,
			PubSubType:  protocol.STATUS,
			StatusCh:    statusListenerQueue,
		}
	}

	//health check sidecar rest api
	healthChk()

	if cfg.EventHandler == types.SOCKET {
		log.Println("Listening to socket")
		SocketListener(&wg, cfg.Socket.Listener.Port, qdrEventInCh)
		log.Println("Creating socket connection to send events")
		socketSenderCon, err = net.DialUDP("udp", nil, &net.UDPAddr{IP: []byte{127, 0, 0, 1}, Port: cfg.Socket.Sender.Port, Zone: ""})
		if err != nil {
			log.Fatal("Error opening socket for ", cfg.Socket.Sender.Port)
		}
		defer socketSenderCon.Close()
	}

	//qdr throws out the data on this channel ,listen to data coming out of qdrEventOutCh
	for { //nolint:gosimple
		select { //nolint:gosimple
		case d := <-qdrEventOutCh: // do something that is put out by QDR
			//Special handle need to redesign
			// ptp status request ,get teh request data ask for ptp socket for data and send it back in its return address
			if d.PubSubType == protocol.STATUS {
				if d.EventStatus == protocol.NEW {
					wg.Add(1)
					go processStatus(&wg, d)
				} else if d.EventStatus == protocol.SUCCEED || d.EventStatus == protocol.FAILED {
					processSuccessStatus(d)
				}
				continue
			}
			// regular subscription events
			data := types.Subscription{}
			err := json.Unmarshal(d.Data.Data(), &data)
			if err != nil {
				log.Printf("Error marshalling event data when reading from QDR %v", err)
			} else {
				// find the endpoint you need to post
				if d.PubSubType == protocol.EVENT { //|always event or status| d.PubSubType == protocol.CONSUMER
					if cfg.EventHandler == types.SOCKET && d.EventStatus == protocol.NEW {
						//now send events from QDR to CNF SOCKET
						if err != nil {
							log.Printf("failed to send events to CNF via socket %v", err)
						} else {
							if err := socketEventToConsumer(data, cfg.Socket.Sender.Port); err != nil {
								log.Printf("error sending to socket %v", err)
							}
						}
					} else { // need to post it via http
						postEventsToConsumer(d)
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

func processSuccessStatus(d protocol.DataEvent) {
	d.Data.SetSpecVersion(cloudevents.VersionV1)
	defer func() {
		if recover() != nil {
			log.Printf("Avoiding panic on channel close")
		}
	}()
	if d.StatusCh != nil {
		data := types.Subscription{}
		err := json.Unmarshal(d.Data.Data(), &data)
		if err != nil {
			log.Printf("Error trying to process status %v", err)
		}
		dataCh := d.StatusCh.GetChannel(data.EventData.SequenceID)
		if dataCh != nil {
			dataCh <- d.Data
		}
	} else {
		log.Printf("GOT Status %v but don't know where to send this", d)
	}

}

func processStatus(wg *sync.WaitGroup, d protocol.DataEvent) {
	defer wg.Done()
	resourceStatus := types.Subscription{}
	err := json.Unmarshal(d.Data.Data(), &resourceStatus)
	resourceStatus.EventData.State = types.ERROR
	if err != nil {
		log.Printf("error marshalling event data when reading from QDR %v", err)
		_ = d.Data.SetData(cloudevents.ApplicationJSON, resourceStatus)
		//if it fails then we cant get return address
	} else {
		//status := checkResourceStatus(wg, "Check ptp status") // check for ptp status
		status := checkPTPStatus()
		if status != "" {
			resourceStatus.EventData.State = types.FREERUN
			resourceStatus.EventData.PTPStatus = status
		}
		_ = d.Data.SetData(cloudevents.ApplicationJSON, resourceStatus)
	}
	// send it back to where it came from
	resourceStatus.ResourceQualifier.SetAddress(resourceStatus.EndpointURI)
	/*sender, err := qdr.NewSender(cfg.AMQP.HostName, cfg.AMQP.Port, resourceStatus.ResourceQualifier.GetAddress())
	if err != nil {
		log.Printf("failed to created sender: %s", resourceStatus.ResourceQualifier.GetAddress())
		return
	}*/
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	//log.Printf("Sending ptp data back %s to %s", status, resourceStatus.ReturnAddress)
	d.Data.SetSpecVersion(cloudevents.VersionV1)
	if result := server.StatusSenders[statusCheckAddress].Client.Send(ctx, d.Data); cloudevents.IsUndelivered(result) {
		log.Printf("failed to send status: %v to addess %s", result, resourceStatus.ResourceQualifier.GetAddress())
	} else if cloudevents.IsNACK(result) {
		log.Printf("status not accepted: %v", result)
	}
	cancel()

}

func postEventsToConsumer(event protocol.DataEvent) {
	// if its consumer then check subscription map TODO
	if event.EventStatus == protocol.NEW { // switch you endpoint url
		//fmt.Sprintf("http://%s:%d%s/event/alert", cnfAPIHostName, cnfAPIPort, "/api/vdu/v1")
		// check for subscription to fetch  the end url
		//TODO: This is required but  adds latency concurrency map
		var eventConsumeURL string
		if v, err := server.GetFromSubStore(event.Address); err == nil {
			eventConsumeURL = v.EndpointURI
		} else {
			log.Printf("Error processing : %v\n", err)
		}
		//eventConsumeURL = fmt.Sprintf("http://%s:%d%s/%s", cfg.Host.HostName, cfg.Host.Port, cfg.HostPathPrefix, "event/alert")
		if eventConsumeURL == "" {
			log.Printf("Could not find publisher/subscription for address %s :", event.Address)
		} else {
			response, err := http.Post(eventConsumeURL, "application/json", bytes.NewBuffer(event.Data.Data()))
			if err != nil {
				log.Printf("publisher failed to post event to consumer %v for url %s", err, event.EndPointURI)
			} else {
				if response.StatusCode != http.StatusAccepted {
					log.Printf("publisher failed to post event to the consumer %s status %d", eventConsumeURL, response.StatusCode)
				}
			}
		}
	} else {
		log.Printf("TODO:// handle  consumer data which is not new(what is that ?) %v", event.EventStatus)
	}
}

func processProducer(event protocol.DataEvent) {
	if event.EventStatus == protocol.SUCCEED {
		if v, err := server.GetFromPubStore(event.Address); err == nil {
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
						log.Printf("failed to send event ack to CNF %d", response.StatusCode)
					}
				}
			}(v)
		} else {
			log.Println(err)
		}
	}
}

func socketEventToConsumer(payload types.Subscription, port int) error {
	//TODO: Change code to keep this connection open
	//Conn, _ := net.DialUDP("udp", nil, &net.UDPAddr{IP: []byte{127, 0, 0, 1}, Port: port, Zone: ""})
	//defer Conn.Close()
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
	if _, err = socketSenderCon.Write(b); err != nil {
		log.Printf("Is there any error in udp %v", err)
		return err
	}
	return nil
}

func healthChk() {
	log.Printf("health check %s ", fmt.Sprintf("http://%s:%d%s/health", cfg.API.HostName, cfg.API.Port, cfg.APIPathPrefix))
	for {
		response, err := http.Get(fmt.Sprintf("http://%s:%d%s/health", cfg.API.HostName, cfg.API.Port, cfg.APIPathPrefix))
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
				PubSubType:  protocol.EVENT,
				EventStatus: 0,
				EndPointURI: sub.EndpointURI,
			}
			dataOut <- d //send to QDR (Now only events are sent via socket , rest happens via http)
		}
	}(wg, udpListenerPort)
}
func checkPTPStatus() string {
	//pmc -u -b 0 "GET CURRENT_DATA_SET" -s /var/run/ptp4l.0.socket
	//pmc -u -b 0 "GET TIME_STATUS_NP" -s /var/run/ptp4l.0.socket
	// pmc -u -b 0 "GET PORT_DATA_SET" -s /var/run/ptp4l.0.socket

	cmdString := []string{"GET CURRENT_DATA_SET", "GET TIME_STATUS_NP", "GET PORT_DATA_SET"}
	cmd := exec.Command("pmc", "-u", "-b 0", cmdString[rand.Int()%len(cmdString)], "-s", "/var/run/ptp4l.0.socket")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("error reading ptp satatus %v", err)
	}
	if string(out) == "" {
		return fmt.Sprintf("error reading ptp satatus %v", err)
	}
	return string(out)
}
func checkResourceStatus(wg *sync.WaitGroup, msg string) types.PTPState { //nolint:deadcode,unused
	r := make(chan types.PTPState)
	f := func(r chan types.PTPState) {
		defer wg.Done()
		c, err := net.Dial("unix", socket.SocketFile)
		if err != nil {
			log.Println("Dial error", err)
			r <- types.ERROR
			return
		}
		defer c.Close()
		_, err = c.Write([]byte(msg))
		if err != nil {
			log.Printf("Write error:%v", err)
			r <- types.ERROR
			return
		}

		buf := make([]byte, 1024)
		//for {
		n, err := c.Read(buf[:])
		if err != nil {
			r <- types.ERROR
			return
		}
		r <- types.PTPState(buf[0:n]) //nolint:staticcheck

	}
	wg.Add(1)
	go f(r)

	select {
	case res := <-r:
		return res
	case <-time.After(500 * time.Millisecond):
		log.Println("out of time :(")
		return types.ERROR
	}

}
