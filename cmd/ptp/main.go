package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	routes3 "github.com/aneeshkp/cloudevents-amqp/pkg/cnf/routes"
	eventconfig "github.com/aneeshkp/cloudevents-amqp/pkg/config"
	"github.com/aneeshkp/cloudevents-amqp/pkg/events"
	"github.com/aneeshkp/cloudevents-amqp/pkg/protocol/rest"
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
	"github.com/gorilla/mux"
	"io/ioutil"
	"log"
	"net/http"
	"sync"

	"math/rand"
	"os"
	"strconv"
	"time"
)

/*
This vdu sends events at random time chosen between 0 to 500ms(configurable in pod)
and send events between 1 to 100K



*/

var (
	//PubStore for storing publisher info that is returned by restapi
	PubStore map[string]types.Subscription

	statusListenerSocketPort = 40001 //nolint:deadcode,unused,varcheck
	avgMessagesPerSec        = 100
	wg                       sync.WaitGroup

	defaultListenerSocketPort = 20001
	defaultSenderSocketPort   = 20002
	defaultAPIPort            = 8080
	defaultHostPort           = 9090
	cfg                       *eventconfig.Config
)

const (
	//SUBROUTINE rest subroutine path
	SUBROUTINE = "/api/vdu/v1"
)

var (
	publisherID string
)

// init sets initial values for variables used in the function.
func init() {
	rand.Seed(time.Now().UnixNano())
}

func main() {
	var err error
	PubStore = map[string]types.Subscription{}
	envMsgPerSec := os.Getenv("MSG_PER_SEC")

	if envMsgPerSec != "" {
		avgMessagesPerSec, _ = strconv.Atoi(envMsgPerSec)
	}
	cfg, err = eventconfig.GetConfig()
	if err != nil {
		log.Printf("Could not load configuration file --config, loading default queue\n")
		cfg = eventconfig.DefaultConfig(defaultHostPort, defaultAPIPort, defaultSenderSocketPort, defaultListenerSocketPort,
			os.Getenv("MY_CLUSTER_NAME"), os.Getenv("MY_NODE_NAME"), os.Getenv("MY_NAMESPACE"), true)
		cfg.EventHandler = types.SOCKET
	}

	//Start the Rest API server to read ack
	wg.Add(1)
	go func() {
		startServer(&wg)
	}()

	// continue after health check
	healthCheckAPIEndpoints()

	publisherID, _ = createPublisher()
	//Latency report
	time.Sleep(5 * time.Second)
	event := events.New(avgMessagesPerSec, PubStore, cfg.Socket.Sender.Port, cfg.EventHandler, PubStore[publisherID])
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
	log.Printf("Health check on rest api's %s", fmt.Sprintf("http://%s:%d%s/health", cfg.API.HostName, cfg.API.Port, rest.SUBROUTINE))
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

func createPublisher() (string, error) {
	nodeName := os.Getenv("MY_NODE_NAME")
	if nodeName == "" {
		nodeName = "unknownnode"
	}

	ptpSubscription := types.Subscription{
		URILocation:  fmt.Sprintf("http://%s:%d%s/event/ack", cfg.Host.HostName, cfg.Host.Port, SUBROUTINE),
		ResourceType: "PTP",
		EndpointURI:  "",
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
	log.Printf("Post URL %s", fmt.Sprintf("http://%s:%d%s/publishers", cfg.API.HostName, cfg.API.Port, rest.SUBROUTINE))
	response, err := http.Post(fmt.Sprintf("http://%s:%d%s/publishers", cfg.API.HostName, cfg.API.Port, rest.SUBROUTINE), "application/json; charset=utf-8", bytes.NewBuffer(jsonValue))
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

//Event will generate random events
func ptpStatusWriteToSocket(wg *sync.WaitGroup) { //nolint:deadcode,unused
	panic("not implemented")
}
