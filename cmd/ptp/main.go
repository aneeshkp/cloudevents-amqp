package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	routes3 "github.com/aneeshkp/cloudevents-amqp/pkg/cnf/routes"
	eventconfig "github.com/aneeshkp/cloudevents-amqp/pkg/config"
	"github.com/aneeshkp/cloudevents-amqp/pkg/events"
	"github.com/aneeshkp/cloudevents-amqp/pkg/socket"
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
	"github.com/gorilla/mux"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
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

	defaultSenderSocketPort   = 20001
	defaultListenerSocketPort = 20002

	defaultAPIPort  = 8080
	defaultHostPort = 9090
	cfg             *eventconfig.Config
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
		cfg.HostPathPrefix = "/api/ptp/v1"
		cfg.APIPathPrefix = "/api/ocloudnotifications/v1"
	}
	log.Printf("Framework type :%s\n", cfg.EventHandler)
	// can override externally
	envEventHandler := os.Getenv("EVENT_HANDLER")
	if envEventHandler != "" {
		cfg.EventHandler = types.EventHandler(envEventHandler)
	}

	//Start the Rest API server to read ack
	wg.Add(1)
	go func() {
		startServer(&wg)
	}()

	// continue after health check
	healthCheckAPIEndpoints()

	publisherID, _ = createPublisher()

	//Start sending events
	event := events.New(avgMessagesPerSec, PubStore, PubStore[publisherID], *cfg)
	time.Sleep(5 * time.Second)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range time.Tick(1 * time.Second) {
			fmt.Printf("|Total message sent mps:|%2.2f|\n", float64(event.GetTotalMsgSent()))
			//atomic.CompareAndSwapUint64(&totalPerSecMsgCount, totalPerSecMsgCount, 0)
			event.ResetTotalMsgSent()
		}
	}()
	//PTP STATUS HANDLER
	if cfg.PublishStatus {
		ptpStatusWriteToSocket(&wg)
	}

	// the PTP has to know where to trigger the events //TODO expose this via ENV
	time.Sleep(5 * time.Second)
	event.GenerateEvents(fmt.Sprintf("http://%s:%d%s/create/event", cfg.API.HostName, cfg.API.Port, cfg.APIPathPrefix), publisherID)
	wg.Wait()
}
func startServer(wg *sync.WaitGroup) {
	defer wg.Done()
	routes := routes3.CnfRoutes{}
	r := mux.NewRouter()
	api := r.PathPrefix(cfg.HostPathPrefix).Subrouter()
	api.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "api v1")
	})
	api.HandleFunc("/publisher/ack", routes.PublisherAck).Methods(http.MethodPost)
	api.HandleFunc("/event/alert", routes.EventSubmit).Methods(http.MethodPost)
	api.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "OK")
	}).Methods(http.MethodGet)
	err := r.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		pathTemplate, err := route.GetPathTemplate()
		if err == nil {
			fmt.Println("ROUTE:", pathTemplate)
		}
		pathRegexp, err := route.GetPathRegexp()
		if err == nil {
			fmt.Println("Path regexp:", pathRegexp)
		}
		queriesTemplates, err := route.GetQueriesTemplates()
		if err == nil {
			fmt.Println("Queries templates:", strings.Join(queriesTemplates, ","))
		}
		queriesRegexps, err := route.GetQueriesRegexp()
		if err == nil {
			fmt.Println("Queries regexps:", strings.Join(queriesRegexps, ","))
		}
		methods, err := route.GetMethods()
		if err == nil {
			fmt.Println("Methods:", strings.Join(methods, ","))
		}
		fmt.Println()
		return nil
	})
	if err != nil {
		fmt.Println(err)
	}
	http.Handle("/", r)

	log.Printf("started rest API server at %s", fmt.Sprintf("http://%s/%d%s", cfg.Host.HostName, cfg.Host.Port, cfg.HostPathPrefix))
	log.Fatal(http.ListenAndServe(fmt.Sprintf("%s:%d", cfg.Host.HostName, cfg.Host.Port), api))
}

//Side car publishers
func healthCheckAPIEndpoints() {
	//health check the webserver
	log.Printf("Health check on hosted services %s", fmt.Sprintf("http://%s:%d%s/health", cfg.Host.HostName, cfg.Host.Port, cfg.HostPathPrefix))
	log.Printf("waiting for self hosted rest service to start\n")
	for {
		response, err := http.Get(fmt.Sprintf("http://%s:%d%s/health", cfg.Host.HostName, cfg.Host.Port, cfg.HostPathPrefix))
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
	log.Printf("waiting for sidecar rest service to start\n")
	for {
		response, err := http.Get(fmt.Sprintf("http://%s:%d%s/health", cfg.API.HostName, cfg.API.Port, cfg.APIPathPrefix))
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
	log.Printf("Ready...")
}

//createPublisher  PTP produces publishers
func createPublisher() (string, error) {
	ptpSubscription := types.Subscription{
		URILocation:  "", //will be filled by the api
		ResourceType: "PTP",
		EndpointURI:  fmt.Sprintf("http://%s:%d%s/publisher/ack", cfg.Host.HostName, cfg.Host.Port, cfg.HostPathPrefix), //return url
		ResourceQualifier: types.ResourceQualifier{
			ClusterName: cfg.Cluster.Name,
			NodeName:    cfg.Cluster.Node,
			Suffix:      []string{"SYNC", "PTP"},
		},
		EventData:      types.EventDataType{},
		EventTimestamp: 0,
		Error:          "",
	}
	jsonValue, _ := json.Marshal(ptpSubscription)
	//Create publisher
	response, err := http.Post(fmt.Sprintf("http://%s:%d%s/publishers", cfg.API.HostName, cfg.API.Port, cfg.APIPathPrefix),
		"application/json; charset=utf-8", bytes.NewBuffer(jsonValue))
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
	wg.Add(1)
	defer wg.Done()
	go socket.SetUpPTPStatusServer(wg)
}
