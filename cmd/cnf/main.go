package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	routes2 "github.com/aneeshkp/cloudevents-amqp/cmd/cnf/routes"
	"github.com/aneeshkp/cloudevents-amqp/pkg/events"
	"github.com/aneeshkp/cloudevents-amqp/pkg/protocol/rest"
	"github.com/aneeshkp/cloudevents-amqp/pkg/report"
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
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

var (
	//PubStore for storing publisher info that is returned by restapi
	PubStore map[string]types.Subscription
	//SubStore for storing Subscription info that is returned by restapi
	SubStore           map[string]types.Subscription
	senderSocketPort   = 30002
	listenerSocketPort = 30001
	avgMessagesPerSec  = 100
	wg                 sync.WaitGroup
	cnfType            = BOTH
	sidCarAPIHostName  = "localhost"
	sideCarAPIPort     = 8080
	apiHostName        = "localhost"
	apiPort            = 9090
	eventHandler       = types.HTTP
)

//CNFTYPE is used to play both roles PTP daemon(PRODUCER) and vDU (CONSUMER)
type CNFTYPE string

const (
	//SUBROUTINE rest subroutine path
	SUBROUTINE = "/api/cnf/v1"
	//PRODUCER specifies role of a CNF
	PRODUCER CNFTYPE = "PRODUCER"
	//CONSUMER specifies role of a CNF
	CONSUMER CNFTYPE = "CONSUMER"
	//BOTH specifies role of a CNF
	BOTH CNFTYPE = "BOTH"
)

var (
	publisherID     string
	eventConsumeURL string
	eventPublishURL string
)

// init sets initial values for variables used in the function.
func init() {
	rand.Seed(time.Now().UnixNano())
}

func main() {
	config()
	PubStore = map[string]types.Subscription{}
	SubStore = map[string]types.Subscription{}

	//Consumer vDU URL event are read from QDR by sidecar and posted to vDU
	eventConsumeURL = fmt.Sprintf("http://%s:%d%s/event/alert", apiHostName, apiPort, SUBROUTINE)
	//SIDECAR URL: PTP events are published via post to its sidecar
	eventPublishURL = fmt.Sprintf("http://%s:%d%s/event/create", sidCarAPIHostName, sideCarAPIPort, rest.SUBROUTINE)
	//Start the Rest API server to read ack
	wg.Add(1)
	go func() {
		startServer(&wg)
	}()

	// continue after health check
	healthCheckAPIEndpoints()

	createPubSub(eventConsumeURL, eventPublishURL) //create publisher or subscription base on configurations
	//Latency report
	time.Sleep(5 * time.Second)

	if eventHandler == types.SOCKET {
		//send count to this channel
		latencyCountCh := make(chan int64, 10)
		latencyReport := report.New(&wg, "default")
		latencyReport.Collect(latencyCountCh)
		socketListener(&wg, "localhost", listenerSocketPort, latencyCountCh)
	}

	event := events.New(avgMessagesPerSec, PubStore, SubStore, senderSocketPort, eventHandler, eventConsumeURL)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range time.Tick(1 * time.Second) {
			fmt.Printf("|Total message sent mps:|%2.2f|\n", float64(event.GetTotalMsgSent()))
			//atomic.CompareAndSwapUint64(&totalPerSecMsgCount, totalPerSecMsgCount, 0)
			event.ResetTotalMsgSent()
		}
	}()
	event.GenerateEvents(publisherID)
}
func startServer(wg *sync.WaitGroup) {
	defer wg.Done()
	routes := routes2.CnfRoutes{}
	r := mux.NewRouter()
	api := r.PathPrefix(SUBROUTINE).Subrouter()
	api.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "api v1")
	})
	api.HandleFunc("/event/ack", routes.EventAck).Methods(http.MethodPost)
	api.HandleFunc("/event/alert", routes.EventAlert).Methods(http.MethodPost)
	api.HandleFunc("/health", routes.Health).Methods(http.MethodGet)
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
		response, err := http.Get(fmt.Sprintf("http://%s:%d%s/health", sidCarAPIHostName, sideCarAPIPort, rest.SUBROUTINE))
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

func createPublisher(eventPublishURL string) (string, error) {
	nodeName := os.Getenv("MY_NODE_NAME")
	if nodeName == "" {
		nodeName = "unknownnode"
	}

	ptpSubscription := types.Subscription{
		URILocation:  fmt.Sprintf("http://%s:%d%s/event/ack", apiHostName, apiPort, SUBROUTINE),
		ResourceType: "PTP",
		EndpointURI:  eventPublishURL,
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
	log.Printf("Post URL %s", fmt.Sprintf("http://%s:%d%s/publishers", sidCarAPIHostName, sideCarAPIPort, rest.SUBROUTINE))
	response, err := http.Post(fmt.Sprintf("http://%s:%d%s/publishers", sidCarAPIHostName, sideCarAPIPort, rest.SUBROUTINE), "application/json; charset=utf-8", bytes.NewBuffer(jsonValue))
	if err != nil {
		log.Printf("create publisher: the http request failed with an error %s\n", err)
		return "", err
	}
	defer response.Body.Close() // Close body only if response non-nil
	if response.StatusCode == http.StatusCreated {
		data, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return "", nil
		}
		var pub types.Subscription
		err = json.Unmarshal(data, &pub)
		if err != nil {
			log.Println("failed to successfully marshal json data from the response")
		} else {
			fmt.Printf("The publisher return is %s\n", string(data))
			PubStore[pub.SubscriptionID] = pub
			return pub.SubscriptionID, nil
		}
	} else {
		err = fmt.Errorf("error returning from http post %d", response.StatusCode)
	}
	return "", err
}

func createSubscription(eventPostURL string) (string, error) {
	nodeName := os.Getenv("MY_NODE_NAME")
	if nodeName == "" {
		nodeName = "unknownnode"
	}
	ptpSubscription := types.Subscription{
		URILocation:  "",
		ResourceType: "PTP",
		EndpointURI:  eventPostURL,
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

	log.Println(fmt.Sprintf("http://%s:%d%s/subscriptions", sidCarAPIHostName, sideCarAPIPort, rest.SUBROUTINE))
	response, err := http.Post(fmt.Sprintf("http://%s:%d%s/subscriptions", sidCarAPIHostName, sideCarAPIPort, rest.SUBROUTINE), "application/json; charset=utf-8", bytes.NewBuffer(jsonValue))
	if err != nil {
		log.Printf("The HTTP request failed with error %s\n", err)
		return "", err
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
			PubStore[sub.SubscriptionID] = sub
			return sub.SubscriptionID, nil
		}
	} else {
		log.Printf("failed to create subscription %d", response.StatusCode)
		err = fmt.Errorf("error returning from http post %d", response.StatusCode)
	}
	return "", err
}

func socketListener(wg *sync.WaitGroup, hostname string, udpListenerPort int, latencyCountChannel chan int64) {
	wg.Add(1)
	go func(wg *sync.WaitGroup, hostname string, udpListenerPort int, latencyCountChannel chan int64) {
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
				log.Printf("socker %v\n", err)
				continue
			}
			//event := cloudevents.NewEvent()
			sub := types.Subscription{}
			err = json.Unmarshal(buffer[:n], &sub)
			if err != nil {
				log.Printf("Error marshalling cloud events")
				continue
			}
			now := time.Now()
			nanos := now.UnixNano()
			// Note that there is no `UnixMillis`, so to get the
			// milliseconds since epoch you'll need to manually
			// divide from nanoseconds.
			millis := nanos / 1000000
			latency := millis - sub.EventTimestamp
			latencyCountChannel <- latency
		}
	}(wg, hostname, udpListenerPort, latencyCountChannel)

}

func config() {
	envMsgPerSec := os.Getenv("MSG_PER_SEC")
	if envMsgPerSec != "" {
		avgMessagesPerSec, _ = strconv.Atoi(envMsgPerSec)
	}

	//determines either to act as event consumer or event publisher
	envCNFType := os.Getenv("CNF_TYPE")
	if envCNFType != "" {
		cnfType = CNFTYPE(envCNFType)
	}
	envSidCarAPIHostName := os.Getenv("SC_API_HOST_NAME")
	if envSidCarAPIHostName != "" {
		sidCarAPIHostName = envSidCarAPIHostName
	}
	envSideCarAPIPort := os.Getenv("SC_API_PORT")
	if envSideCarAPIPort != "" {
		sideCarAPIPort, _ = strconv.Atoi(envSideCarAPIPort)
	}
	envAPIHostName := os.Getenv("API_HOST_NAME")
	if envAPIHostName != "" {
		apiHostName = envAPIHostName
	}
	envAPIPort := os.Getenv("API_PORT")
	if envAPIPort != "" {
		apiPort, _ = strconv.Atoi(envAPIPort)
	}
}

func createPubSub(eventConsumer string, eventPublishURL string) {
	if cnfType == PRODUCER {
		log.Printf("Creating default Publishers")
		_, _ = createPublisher(eventPublishURL)
	} else if cnfType == CONSUMER {
		log.Printf("Creating default Subscriptions")
		_, _ = createSubscription(eventConsumeURL)
	} else if cnfType == BOTH {
		log.Printf("Creating default Subscriptions")
		_, _ = createSubscription(eventConsumeURL)
		log.Printf("Creating default Publishers")
		publisherID, _ = createPublisher(eventPublishURL)
	} else {
		log.Println("Stop.Cannot detect if cnf is Producer or Consumer.")
		return
	}

}
