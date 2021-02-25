package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	routes3 "github.com/aneeshkp/cloudevents-amqp/pkg/cnf/routes"
	eventconfig "github.com/aneeshkp/cloudevents-amqp/pkg/config"
	"github.com/aneeshkp/cloudevents-amqp/pkg/protocol/rest"
	"github.com/aneeshkp/cloudevents-amqp/pkg/report"
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
	"time"
)

/*
This vdu sends events at random time chosen between 0 to 500ms(configurable in pod)
and send events between 1 to 100K



*/

var (
	//SubStore for storing Subscription info that is returned by restapi
	SubStore                  map[string]types.Subscription
	wg                        sync.WaitGroup
	cfg                       *eventconfig.Config
	eventConsumeURL           string
	defaultListenerSocketPort = 3001
	defaultSenderSocketPort   = 3002
	defaultAPIPort            = 8081
	defaultHostPort           = 9091
)

const (
	//SUBROUTINE rest subroutine path
	SUBROUTINE = "/api/vdu/v1"
)

// init sets initial values for variables used in the function.
func init() {
	rand.Seed(time.Now().UnixNano())
}

func main() {
	var err error
	cfg, err = eventconfig.GetConfig()
	if err != nil {
		log.Printf("Could not load configuration file --config, loading default queue\n")
		cfg = eventconfig.DefaultConfig(defaultHostPort, defaultAPIPort, defaultSenderSocketPort, defaultListenerSocketPort,
			os.Getenv("MY_CLUSTER_NAME"), os.Getenv("MY_NODE_NAME"), os.Getenv("MY_NAMESPACE"), false)
		cfg.EventHandler = types.SOCKET
	}
	SubStore = map[string]types.Subscription{}

	//Consumer vDU URL event are read from QDR by sidecar and posted to vDU
	eventConsumeURL = fmt.Sprintf("http://%s:%d%s/event/alert", cfg.Host.HostName, cfg.Host.Port, SUBROUTINE)
	//SIDECAR URL: PTP events are published via post to its sidecar
	//eventPublishURL = fmt.Sprintf("http://%s:%d%s/event/create", cfg.API.HostName, cfg.API.Port, rest.SUBROUTINE)
	//Start the Rest API server to read ack
	wg.Add(1)
	go func() {
		startServer(&wg)
	}()

	// continue after health check
	healthCheckAPIEndpoints()

	// create various subscriptions that you are interested to subscribe to
	_, _ = createSubscription(eventConsumeURL)

	time.Sleep(5 * time.Second)

	// check status of PTP
	tck := time.NewTicker(time.Duration(10) * time.Second)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range tck.C {
			event, err := checkPTPStatus()
			if err != nil {
				log.Printf("error check ptp status %v\n", err)
			}
			log.Printf("PTP status %v\n", string(event.Data()))
		}
	}()

	if cfg.EventHandler == types.SOCKET {
		//send count to this channel
		latencyCountCh := make(chan int64, 10)
		latencyReport := report.New(&wg, "default")
		latencyReport.Collect(latencyCountCh)
		socketListener(&wg, cfg.Socket.Listener.HostName, cfg.Socket.Listener.Port, latencyCountCh)
	}

	wg.Wait()
}
func startServer(wg *sync.WaitGroup) {
	defer wg.Done()
	routes := routes3.CnfRoutes{}
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
	log.Fatal(http.ListenAndServe(fmt.Sprintf("%s:%d", cfg.Host.HostName, cfg.Host.Port), api))
}

//Side car publishers
func healthCheckAPIEndpoints() {
	//health check the webserver
	log.Printf("Health check on hosted services %s", fmt.Sprintf("http://%s:%d%s/health", cfg.Host.HostName, cfg.Host.Port, SUBROUTINE))
	log.Printf("waiting for self hosted rest service to start\n")
	for {
		response, err := http.Get(fmt.Sprintf("http://%s:%d%s/health", cfg.Host.HostName, cfg.Host.Port, SUBROUTINE))
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		if response != nil && response.StatusCode == http.StatusOK {
			response.Body.Close()
			break
		}
		response.Body.Close()
		time.Sleep(2 * time.Second)
	}

	//health check sidecar rest api
	log.Printf("health check on rest api's %s", fmt.Sprintf("http://%s:%d%s/health", cfg.API.HostName, cfg.API.Port, rest.SUBROUTINE))
	log.Printf("waiting for sidecar reest rest service to start\n")
	for {
		response, err := http.Get(fmt.Sprintf("http://%s:%d%s/health", cfg.API.HostName, cfg.API.Port, rest.SUBROUTINE))
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		if response != nil && response.StatusCode == http.StatusOK {
			response.Body.Close()
			break
		}
		response.Body.Close()
		time.Sleep(2 * time.Second)
	}
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

	log.Println(fmt.Sprintf("http://%s:%d%s/subscriptions", cfg.API.HostName, cfg.API.Port, rest.SUBROUTINE))
	response, err := http.Post(fmt.Sprintf("http://%s:%d%s/subscriptions", cfg.API.HostName, cfg.API.Port, rest.SUBROUTINE), "application/json; charset=utf-8", bytes.NewBuffer(jsonValue))
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
			SubStore[sub.SubscriptionID] = sub
			return sub.SubscriptionID, nil
		}
	} else {
		log.Printf("failed to create subscription %d", response.StatusCode)
		err = fmt.Errorf("error returning from http post %d", response.StatusCode)
	}
	return "", err
}

func socketListener(wg *sync.WaitGroup, hostname string, listenerPort int, latencyCountChannel chan int64) {
	wg.Add(1)
	go func(wg *sync.WaitGroup, hostname string, udpListenerPort int, latencyCountChannel chan int64) {
		defer wg.Done()
		ServerConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: []byte{0, 0, 0, 0}, Port: listenerPort, Zone: ""})
		if err != nil {
			log.Fatalf("Error setting up socket %v", err)
		}
		defer ServerConn.Close()
		buffer := make([]byte, 1024)
		for {
			n, _, err := ServerConn.ReadFromUDP(buffer)
			if err != nil {
				log.Printf("socket %v\n", err)
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
	}(wg, hostname, listenerPort, latencyCountChannel)

}

func checkPTPStatus() (event cloudevents.Event, err error) {

	log.Printf("Posting to PTP status %s\n", fmt.Sprintf("http://%s:%d%s/status", cfg.API.HostName, cfg.API.Port, rest.SUBROUTINE))
	/*client := http.Client{
		Timeout: 10 * time.Second,
	}*/
	response, err := http.Post(fmt.Sprintf("http://%s:%d%s/status", cfg.API.HostName, cfg.API.Port, rest.SUBROUTINE),
		"application/json; charset=utf-8", nil)
	if err != nil {
		log.Printf("The HTTP request for PTP status failed with error %s\n", err)
		return
	}
	defer response.Body.Close() // Close body only if response non-nil
	if response.StatusCode == http.StatusOK {
		defer response.Body.Close() // Close body only if response non-nil
		data, _ := ioutil.ReadAll(response.Body)
		log.Printf("what is in the PTP data %v\n", string(data))
		err = json.Unmarshal(data, &event)
		if err != nil {
			log.Println("failed to successfully marshal ptp status json data from the response")
		} else {
			log.Printf("Got status %s", string(data))
		}
	} else {
		log.Printf("failed to create PTP status %d", response.StatusCode)
		err = fmt.Errorf("error returning from http post %d", response.StatusCode)
	}
	return
}
