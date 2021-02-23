package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/aneeshkp/cloudevents-amqp/pkg/chain"
	"github.com/aneeshkp/cloudevents-amqp/pkg/protocol/rest"
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/gorilla/mux"
	"io/ioutil"
	"log"
	"net/http"
	"sync"

	"math/rand"
	"net"
	"os"
	"strconv"
	"time"
)

/*
This cnf sends events at random time chosen between 0 to 500ms(configurable in pod)
and send events between 1 to 100K



*/
const (
	maxBinSize = 1002
)

var (
	useSocket                  = true
	udpSenderPort              = 20002
	udpListenerPort            = 20001
	totalPerSecMsgCount uint64 = 0
	avgMessagesPerSec          = 100
	wg                  sync.WaitGroup
	PubStore            map[string]types.Subscription
	SubStore            map[string]types.Subscription
	cnfType             = BOTH
	sidCarApiHostName   = "localhost"
	sideCarApiPort      = 8080
	apiHostName         = "localhost"
	apiPort             = 9090
	eventHandler        = types.HTTP
	latencyResult       map[string]*latency
	latencyResults      []int
	//channelBufferSize      = bufferSize
	binSize = maxBinSize

	globalMsgReceivedCount int64
)

type LCount struct {
	Time int64 `json: "time,omitempty,string"`
}
type latency struct {
	ID       string
	MsgCount int64
	Latency  [maxBinSize]int64
}
type CNFTYPE string

const (
	SUBROUTINE         = "/api/cnf/v1"
	PRODUCER   CNFTYPE = "PRODUCER"
	CONSUMER   CNFTYPE = "CONSUMER"
	BOTH       CNFTYPE = "BOTH"
)

var (
	eventAckEndpoint = fmt.Sprintf("%s/%s", SUBROUTINE, "event")
	pubAckEndpoint   = fmt.Sprintf("%s/%s", SUBROUTINE, "publisher")
	subAckEndpoint   = fmt.Sprintf("%s/%s", SUBROUTINE, "subscription")
)

// init sets initial values for variables used in the function.
func init() {
	rand.Seed(time.Now().UnixNano())
}
func startServer(wg *sync.WaitGroup) {
	defer wg.Done()

	r := mux.NewRouter()
	api := r.PathPrefix(SUBROUTINE).Subrouter()
	api.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "api v1")
		fmt.Fprintln(w, fmt.Sprintf("%s%s", SUBROUTINE, "/event/ack"))
		fmt.Fprintln(w, fmt.Sprintf("%s%s", SUBROUTINE, "/event/alert"))
		fmt.Fprintln(w, fmt.Sprintf("%s%s", SUBROUTINE, "/health"))
	})
	api.HandleFunc("/event/ack", eventAck).Methods(http.MethodPost)
	api.HandleFunc("/event/alert", eventAlert).Methods(http.MethodPost)
	api.HandleFunc("/health", health).Methods(http.MethodGet)
	log.Print("Started Rest API Server")
	log.Printf("endpoint %s", SUBROUTINE)
	log.Fatal(http.ListenAndServe(fmt.Sprintf("%s:%d", apiHostName, apiPort), api))
}

//Side car publishers
func healthCheckAPIEndpoints() {
	//health check the webserver
	for {
		response, err := http.Get(fmt.Sprintf("http://%s:%d%s/health", apiHostName, apiPort, SUBROUTINE))
		if err != nil {
			log.Printf("Error connecting to rest api %v", err)
			continue
		}
		if response != nil && response.StatusCode == http.StatusOK {
			response.Body.Close()
			break
		}
		response.Body.Close()
		log.Printf("waiting for self hosted rest service to start\n")
		time.Sleep(2 * time.Second)
	}

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

}

func main() {
	PubStore = map[string]types.Subscription{}
	SubStore = map[string]types.Subscription{}
	//Start the Rest API server to read ack
	wg.Add(1)
	go func() {
		startServer(&wg)
	}()
	//determines either to act as event consumer or event publisher
	envCNFType := os.Getenv("CNF_TYPE")
	if envCNFType != "" {
		cnfType = CNFTYPE(envCNFType)
	}
	envSidCarApiHostName := os.Getenv("SC_API_HOST_NAME")
	if envSidCarApiHostName != "" {
		sidCarApiHostName = envSidCarApiHostName
	}
	envSideCarApiPort := os.Getenv("SC_API_PORT")
	if envSideCarApiPort != "" {
		sideCarApiPort, _ = strconv.Atoi(envSideCarApiPort)
	}
	envApiHostName := os.Getenv("API_HOST_NAME")
	if envApiHostName != "" {
		apiHostName = envApiHostName
	}
	envApiPort := os.Getenv("API_PORT")
	if envApiPort != "" {
		apiPort, _ = strconv.Atoi(envApiPort)
	}

	// continue after health check
	//var subscriptionID string
	var publisherID string

	healthCheckAPIEndpoints()
	if cnfType == PRODUCER {
		log.Printf("Creating default Publishers")
		_, _ = createPublisher()
	} else if cnfType == CONSUMER {
		log.Printf("Creating default Subscriptions")
		_, _ = createSubscription()
	} else if cnfType == BOTH {
		log.Printf("Creating default Subscriptions")
		_, _ = createSubscription()
		log.Printf("Creating default Publishers")
		publisherID, _ = createPublisher()
	} else {
		log.Println("Stop.Cannot detect if cnf is Producer or Consumer.")
		return
	}

	latencyResult = make(map[string]*latency, 1)
	latencyResults = make([]int, binSize)
	if eventHandler == types.UDP {
		wg.Add(1)
		go udpListenerSetup(&wg, "localhost", udpListenerPort)

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

	states := intiState()
	envMsgPerSec := os.Getenv("MSG_PER_SEC")
	if envMsgPerSec != "" {
		avgMessagesPerSec, _ = strconv.Atoi(envMsgPerSec)
	}

	transition := [][]float32{
		{
			0.6, 0.1, 0.1, 0.0, 0.2,
		},
		{
			0.1, 0.7, 0.1, 1.0, 1.0,
		},
		{
			0.3, 0.3, 0.1, 0.1, 0.0,
		},
		{
			0.2, 0.3, 0.3, 0.1, 0.1,
		},
		{
			0.3, 0.3, 0.3, 0.1, 0.0,
		},
	}

	fmt.Printf("Sleeping %d sec...\n", 10)
	//time.Sleep(time.Duration() * time.Second)
	//diceTicker := time.NewTicker(time.Duration(rollInMsMin) * time.Millisecond)
	//avgPerSecTicker := time.NewTicker(time.Duration(1) * time.Second)

	// initialize current state
	currentStateID := 1
	c := chain.Create(transition, states)
	currentStateChoice := c.GetStateChoice(currentStateID)
	currentStateID = currentStateChoice.Item.(int)

	avgMsgPeriodMs := 1000 / avgMessagesPerSec //100
	fmt.Printf("avgMsgPerMs: %d\n", avgMsgPeriodMs)
	midpoint := avgMsgPeriodMs / 2

	fmt.Printf("midpoint: %d\n", midpoint)

	if eventHandler == types.HTTP {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range time.Tick(1 * time.Second) {
				fmt.Printf("|Total message sent mps:|%2.2f|\n", float64(totalPerSecMsgCount))
				//atomic.CompareAndSwapUint64(&totalPerSecMsgCount, totalPerSecMsgCount, 0)
				totalPerSecMsgCount = 0
			}
		}()
	}

	tck := time.NewTicker(time.Duration(1000) * time.Microsecond)
	maxCount := avgMsgPeriodMs
	counter := 0
	for range tck.C {
		currentStateChoice := c.GetStateChoice(currentStateID)
		currentStateID = currentStateChoice.Item.(int)
		currentState := c.GetState(currentStateID)
		if counter >= maxCount {
			if currentState.Probability > 0 {
				//for i := 1; i <= currentState.Payload.ID; i++ {
				if eventHandler == types.UDP {

					err := udpEvent(currentState.Payload, publisherID)
					if err == nil {
						totalPerSecMsgCount++
					}
				} else {
					err := httpEvent(currentState.Payload, publisherID)
					if err == nil {
						totalPerSecMsgCount++
					}
				}
			}
			maxCount = rand.Intn(avgMsgPeriodMs+1) + midpoint
			counter = 0
		}
		counter++
	}

}

func intiState() []chain.State {
	//events := getSupportedEvents()
	nodeName := os.Getenv("MY_NODE_NAME")
	if nodeName == "" {
		nodeName = "unknownnode"
	}
	states := []chain.State{
		{
			ID: 1,
			Payload: types.Subscription{
				SubscriptionId: "",
				UriLocation:    "",
				ResourceType:   "",
				EndpointUri:    "",
				ResourceQualifier: types.ResourceQualifier{
					NodeName:    nodeName,
					NameSpace:   os.Getenv("MY_POD_NAMESPACE"),
					ClusterName: "unknown",
					Suffix:      []string{"SYNC", "PTP"},
				},
				EventData:      types.EventDataType{State: types.FREERUN},
				EventTimestamp: 0,
				Error:          "",
			},
			Probability: 99,
		},
		{
			ID: 2,
			Payload: types.Subscription{
				SubscriptionId: "",
				UriLocation:    "",
				ResourceType:   "",
				EndpointUri:    "",
				ResourceQualifier: types.ResourceQualifier{
					NodeName:    nodeName,
					NameSpace:   os.Getenv("MY_POD_NAMESPACE"),
					ClusterName: "unknown",
					Suffix:      []string{"SYNC", "PTP"},
				},
				EventData:      types.EventDataType{State: types.HOLDOVER},
				EventTimestamp: 0,
				Error:          "",
			},
			Probability: 99,
		},
		{
			ID: 3,
			Payload: types.Subscription{
				SubscriptionId: "",
				UriLocation:    "",
				ResourceType:   "",
				EndpointUri:    "",
				ResourceQualifier: types.ResourceQualifier{
					NodeName:    nodeName,
					NameSpace:   os.Getenv("MY_POD_NAMESPACE"),
					ClusterName: "unknown",
					Suffix:      []string{"SYNC", "PTP"},
				},
				EventData:      types.EventDataType{State: types.LOCKED},
				EventTimestamp: 0,
				Error:          "",
			},
			Probability: 99,
		},
		{
			ID: 4,
			Payload: types.Subscription{
				SubscriptionId: "",
				UriLocation:    "",
				ResourceType:   "",
				EndpointUri:    "",
				ResourceQualifier: types.ResourceQualifier{
					NodeName:    nodeName,
					NameSpace:   os.Getenv("MY_POD_NAMESPACE"),
					ClusterName: "unknown",
					Suffix:      []string{"SYNC", "PTP"},
				},
				EventData:      types.EventDataType{State: types.FREERUN},
				EventTimestamp: 0,
				Error:          "",
			},
			Probability: 99,
		},
		{
			ID: 5,
			Payload: types.Subscription{
				SubscriptionId: "",
				UriLocation:    "",
				ResourceType:   "",
				EndpointUri:    "",
				ResourceQualifier: types.ResourceQualifier{
					NodeName:    nodeName,
					NameSpace:   os.Getenv("MY_POD_NAMESPACE"),
					ClusterName: "unknown",
					Suffix:      []string{"SYNC", "PTP"},
				},
				EventData:      types.EventDataType{State: types.LOCKED},
				EventTimestamp: 0,
				Error:          "",
			},
			Probability: 99,
		},
	}
	return states
}

//Event will generate random events
func udpEvent(payload types.Subscription, publisherID string) error {
	if pub, ok := PubStore[publisherID]; ok {
		payload.SubscriptionId = publisherID
		payload.UriLocation = pub.UriLocation
		payload.EndpointUri = fmt.Sprintf("%s:%d%s/event/ack", "http://localhost", 9090, SUBROUTINE)
		now := time.Now()
		nanos := now.UnixNano()
		// Note that there is no `UnixMillis`, so to get the
		// milliseconds since epoch you'll need to manually
		// divide from nanoseconds.
		millis := nanos / 1000000
		payload.EventTimestamp = millis

		b, err := json.Marshal(payload)
		if err != nil {
			log.Printf("The marshal error %s\n", err)
			return err
		}
		Conn, _ := net.DialUDP("udp", nil, &net.UDPAddr{IP: []byte{127, 0, 0, 1}, Port: udpSenderPort, Zone: ""})
		defer Conn.Close()

		if _, err = Conn.Write(b); err != nil {
			return err
		}

		if err != nil {
			log.Printf("failed to send  event via UDP %s\n", err)
		}
	}
	//fmt.Printf("Sending %v messages\n", payload)
	return nil

}

//Event will generate random events
func httpEvent(payload types.Subscription, publisherID string) error {
	if pub, ok := PubStore[publisherID]; ok {
		payload.SubscriptionId = publisherID
		payload.UriLocation = pub.UriLocation
		payload.EndpointUri = fmt.Sprintf("%s:%d%s/event/ack", "http://localhost", 9090, SUBROUTINE)
		now := time.Now()
		nanos := now.UnixNano()
		// Note that there is no `UnixMillis`, so to get the
		// milliseconds since epoch you'll need to manually
		// divide from nanoseconds.
		millis := nanos / 1000000
		payload.EventTimestamp = millis

		payload, err := json.Marshal(payload)
		if err != nil {
			log.Printf("The marshal error %s\n", err)
		}

		//log.Printf("Posting event to %s\n", pub.EndpointUri)
		response, err := http.Post(pub.EndpointUri, "application/json", bytes.NewBuffer(payload))
		if err != nil {
			log.Printf("The HTTP request failed with error %s\n", err)
			return err

		} else if response.StatusCode == http.StatusAccepted {
			log.Printf("HttP Event sent....")
		}
	}
	//fmt.Printf("Sending %v messages\n", payload)
	return nil
}

func createPublisher() (string, error) {
	nodeName := os.Getenv("MY_NODE_NAME")
	if nodeName == "" {
		nodeName = "unknownnode"
	}

	ptpSubscription := types.Subscription{
		UriLocation:  fmt.Sprintf("http://%s:%d%s/event/ack", apiHostName, apiPort, SUBROUTINE),
		ResourceType: "PTP",
		EndpointUri:  "",
		ResourceQualifier: types.ResourceQualifier{
			NodeName:    nodeName,
			ClusterName: "unknown",
			Suffix:      []string{"SYNC", "PTP"},
		},
		EventData:      types.EventDataType{},
		EventTimestamp: 0,
		Error:          "",
	}
	jsonValue, _ := json.Marshal(ptpSubscription)
	log.Printf("Post URL %s", fmt.Sprintf("http://%s:%d%s/publishers", sidCarApiHostName, sideCarApiPort, rest.SUBROUTINE))
	response, err := http.Post(fmt.Sprintf("http://%s:%d%s/publishers", sidCarApiHostName, sideCarApiPort, rest.SUBROUTINE), "application/json; charset=utf-8", bytes.NewBuffer(jsonValue))
	if err != nil {
		log.Printf("The HTTP request failed with error %s\n", err)
		return "",err
	}
	defer response.Body.Close() // Close body only if response non-nil
	if response.StatusCode == http.StatusCreated {
		data, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return "",nil
		}
		var pub types.Subscription
		err = json.Unmarshal(data, &pub)
		if err != nil {
			log.Println("failed to successfully marshal json data from the response")
		} else {
			fmt.Printf("The publisher return is %s\n", string(data))
			PubStore[pub.SubscriptionId] = pub
			return pub.SubscriptionId, nil
		}
	} else {
		err = fmt.Errorf("error returning from http post %d", response.StatusCode)
	}
	return "", err
}
func createSubscription() (string, error) {
	nodeName := os.Getenv("MY_NODE_NAME")
	if nodeName == "" {
		nodeName = "unknownnode"
	}
	ptpSubscription := types.Subscription{
		UriLocation:  fmt.Sprintf("http://%s:%d%s/event/alert", apiHostName, apiPort, SUBROUTINE),
		ResourceType: "PTP",
		EndpointUri:  fmt.Sprintf("http://%s:%d%s/event/alert", apiHostName, apiPort, SUBROUTINE),
		ResourceQualifier: types.ResourceQualifier{
			NodeName:    nodeName,
			ClusterName: "unknown",
			Suffix:      []string{"SYNC", "PTP"},
		},
		EventData:      types.EventDataType{},
		EventTimestamp: 0,
		Error:          "",
	}
	jsonValue, _ := json.Marshal(ptpSubscription)

	log.Println(fmt.Sprintf("http://%s:%d%s/subscriptions", sidCarApiHostName, sideCarApiPort, rest.SUBROUTINE))
	response, err := http.Post(fmt.Sprintf("http://%s:%d%s/subscriptions", sidCarApiHostName, sideCarApiPort, rest.SUBROUTINE), "application/json; charset=utf-8", bytes.NewBuffer(jsonValue))
	if err != nil {
		log.Printf("The HTTP request failed with error %s\n", err)
		return "",err
	}
	defer response.Body.Close() // Close body only if response non-nil
	if response.StatusCode == http.StatusCreated {
		defer response.Body.Close() // Close body only if response non-nil
		data, _ := ioutil.ReadAll(response.Body)
		var sub types.Subscription
		err = json.Unmarshal(data, &sub)
		if err != nil {
			log.Println("failed to successfully marshal json data from the response")
		} else {
			log.Printf("The subscription return is %v", sub)
			PubStore[sub.SubscriptionId] = sub
			return sub.SubscriptionId, nil
		}
	} else {
		log.Printf("failed to create subscription %d", response.StatusCode)
		err = fmt.Errorf("error returning from http post %d", response.StatusCode)
	}
	return "", err
}

func eventAck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	queries := mux.Vars(r)
	eventID, ok := queries["eventid"]
	if ok {
		log.Println(eventID)
	}
	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte("ok"))
}

func eventAlert(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	defer r.Body.Close()
	bodyBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "reading request"}`))
		return
	}
	sub := types.Subscription{}
	if err := json.Unmarshal(bodyBytes, &sub); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "reading subscription data "}`))
		return
	}
	now := time.Now()
	nanos := now.UnixNano()
	// Note that there is no `UnixMillis`, so to get the
	// milliseconds since epoch you'll need to manually
	// divide from nanoseconds.
	millis := nanos / 1000000

	w.WriteHeader(http.StatusAccepted)
	latency := millis - sub.EventTimestamp

	w.Write([]byte(fmt.Sprintf(`{"time":%d}`, latency)))
}

func health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func udpListenerSetup(wg *sync.WaitGroup, hostname string, udpListenerPort int) {
	defer wg.Done()
	ServerConn, _ := net.ListenUDP("udp", &net.UDPAddr{IP: []byte{0, 0, 0, 0}, Port: udpListenerPort, Zone: ""})
	defer ServerConn.Close()
	buffer := make([]byte, 1024)
	for {
		n, _, err := ServerConn.ReadFromUDP(buffer)
		if err != nil {
			continue
		}
		//event := cloudevents.NewEvent()
		sub := types.Subscription{}
		err = json.Unmarshal(buffer[:n], &sub)
		if err != nil {
			log.Printf("Error marshalling cloud events")
			return
		}

		now := time.Now()
		nanos := now.UnixNano()
		// Note that there is no `UnixMillis`, so to get the
		// milliseconds since epoch you'll need to manually
		// divide from nanoseconds.
		millis := nanos / 1000000
		latency := millis - sub.EventTimestamp
		if latency >= int64(binSize) {
			latency = int64(binSize - 1)
		}
		latencyResults[latency]++
		globalMsgReceivedCount++

	}

}

// Listener always listens to events only
func handleUDPConnection(conn *net.UDPConn) {
	buffer := make([]byte, 1500)
	n, _, err := conn.ReadFromUDP(buffer)
	if err != nil {
		//log.Print(err)
		return
	}
	event := cloudevents.NewEvent()

	sub := types.Subscription{}
	err = json.Unmarshal(buffer[:n], &event)
	if err != nil {
		log.Printf("Error marshalling cloud events")
		return
	}
	err = json.Unmarshal(event.Data(), &sub)
	if err != nil {
		log.Printf("Error marshalling ptp events ")
		return
	}
	now := time.Now()
	nanos := now.UnixNano()
	// Note that there is no `UnixMillis`, so to get the
	// milliseconds since epoch you'll need to manually
	// divide from nanoseconds.
	millis := nanos / 1000000
	latency := millis - sub.EventTimestamp
	if latency >= int64(binSize) {
		latency = int64(binSize - 1)
	}
	latencyResults[latency]++
	globalMsgReceivedCount++

}
