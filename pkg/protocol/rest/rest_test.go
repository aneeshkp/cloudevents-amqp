package rest_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/aneeshkp/cloudevents-amqp/pkg/protocol"
	"github.com/aneeshkp/cloudevents-amqp/pkg/protocol/qdr"
	"github.com/aneeshkp/cloudevents-amqp/pkg/protocol/rest"
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"testing"
	"time"
)

var (
	server  *rest.Server
	router  *qdr.Router
	eventCh chan protocol.DataEvent
	wg      sync.WaitGroup
)

func init() {
	eventCh = make(chan protocol.DataEvent, 10)
}

func TestServer_New(t *testing.T) {

	// have one receiver for testing
	router = qdr.InitServer("amqp://localhost", 5672)
	router.DataIn = eventCh

	wg.Add(1)
	// create a receiver
	err := router.NewReceiver("test")
	err = router.NewReceiver("test2")
	if err != nil {
		t.Errorf("assert  error; %v ", err)
	}

	go router.Receive(&wg, "test", func(e cloudevents.Event) {
		log.Printf("Received event  %s", string(e.Data()))
	})
	go router.Receive(&wg, "test2", func(e cloudevents.Event) {
		log.Printf("Received event  %s", string(e.Data()))
	})

	//Sender sitting and waiting either to send or receive just create address or create address and send or receive
	go router.QDRRouter(&wg)

	server = rest.InitServer("localhost", 8080, "emptydir/pub.json", "emptydir/sub.json")
	server.DataOut = eventCh
	//start http server
	wg.Add(1)
	go func() {
		defer wg.Done()
		server.Start()
	}()

	time.Sleep(3 * time.Second)
	// this should actually send an event

	// CHECK URL IS UP
	req, err := http.NewRequest("GET", "http://localhost:8080/api/ocloudnotifications/v1/health", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.HttpClient.Do(req)
	if err != nil {
		panic(err)
	}
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// create subscription
	sub := types.Subscription{
		SubscriptionId: "",
		UriLocation:    "",
		ResourceType:   "PTP",
		EndpointUri:    "http://localhost:8080/api/ocloudnotifications/v1/suback",
		ResourceQualifier: types.ResourceQualifier{
			NodeName:    "TestNode",
			ClusterName: "TestCluster",
			Suffix:      []string{"abc", "xyz"},
		},
		EventData:      types.EventDataType{State: types.FREERUN},
		EventTimestamp: 0,
	}
	data, err :=  json.Marshal(&sub)
	assert.Nil(t, err)
	assert.NotNil(t, data)
	resp.Body.Close()
	/// create new subscription
	req, err = http.NewRequest("POST", "http://localhost:8080/api/ocloudnotifications/v1/subscriptions", bytes.NewBuffer(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err = server.HttpClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	bodyString := string(bodyBytes)
	log.Print(bodyString)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	err = json.Unmarshal(bodyBytes, &sub)
	assert.Nil(t, err)

	// Get Just Created Subscription
	req, err = http.NewRequest("GET", fmt.Sprintf("http://localhost:8080/api/ocloudnotifications/v1/subscriptions/%s", sub.SubscriptionId), nil)
	req.Header.Set("Content-Type", "application/json")
	resp, err = server.HttpClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	bodyBytes, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	var rSub types.Subscription
	err = json.Unmarshal(bodyBytes, &rSub)
	if e, ok := err.(*json.SyntaxError); ok {
		log.Printf("syntax error at byte offset %d", e.Offset)
	}
	bodyString = string(bodyBytes)
	log.Print(bodyString)
	assert.Nil(t, err)
	assert.Equal(t, sub.SubscriptionId, rSub.SubscriptionId)
	resp.Body.Close()

	// Get All Subscriptions
	req, err = http.NewRequest("GET", "http://localhost:8080/api/ocloudnotifications/v1/subscriptions", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, err = server.HttpClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close() // Close body only if response non-nil
	bodyBytes, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	var subList []types.Subscription
	log.Println(string(bodyBytes))
	err = json.Unmarshal(bodyBytes, &subList)
	assert.Nil(t, err)
	assert.Greater(t, len(subList), 0)

	//********************Publisher
	// create subscription
	// create subscription
	pub := types.Subscription{
		SubscriptionId: "",
		UriLocation:    "",
		ResourceType:   "PTP",
		EndpointUri:    "http://localhost:8080/api/ocloudnotifications/v1/suback",
		ResourceQualifier: types.ResourceQualifier{
			NodeName:    "TestNode",
			ClusterName: "TestCluster",
			Suffix:      []string{"abc", "xyz"},
		},
		EventData:      types.EventDataType{State: types.FREERUN},
		EventTimestamp: 0,
	}
	pubData, err :=  json.Marshal(&pub)
	assert.Nil(t, err)
	assert.NotNil(t, pubData)

	req, err = http.NewRequest("POST", "http://localhost:8080/api/ocloudnotifications/v1/publishers", bytes.NewBuffer(pubData))
	req.Header.Set("Content-Type", "application/json")
	resp, err = server.HttpClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	pubBodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	err = json.Unmarshal(pubBodyBytes, &pub)
	assert.Nil(t, err)

	pubBodyString := string(pubBodyBytes)
	log.Print(pubBodyString)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Get Just created Publisher
	req, err = http.NewRequest("GET", fmt.Sprintf("http://localhost:8080/api/ocloudnotifications/v1/publishers/%s", pub.SubscriptionId), nil)
	req.Header.Set("Content-Type", "application/json")
	resp, err = server.HttpClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	pubBodyBytes, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	var rPub types.Subscription
	log.Printf("the data %s",string(pubBodyBytes))
	err = json.Unmarshal(pubBodyBytes, &rPub)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	assert.Nil(t, err)
	assert.Equal(t, pub.SubscriptionId, rPub.SubscriptionId)
	resp.Body.Close()

	// Get All Publisher
	req, err = http.NewRequest("GET", "http://localhost:8080/api/ocloudnotifications/v1/publishers", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, err = server.HttpClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	pubBodyBytes, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	var pubList []types.Subscription
	err = json.Unmarshal(pubBodyBytes, &pubList)
	assert.Nil(t, err)
	assert.Greater(t, len(pubList), 0)

	/*
		r, _ = http.NewRequest("GET", "http://localhost:8080/api/v1/addresses", nil)
		resp, err = client.Do(r)
		if err != nil {
			panic(err)
		}
		defer resp.Body.Close()
		bodyBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Fatal(err)
		}
		bodyString := string(bodyBytes)
		log.Print(bodyString)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

	*/

	// Delete All Publisher
	/*req, err = http.NewRequest("DELETE", "http://localhost:8080/api/ocloudnotifications/v1/publishers", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, err = server.HttpClient.Do(req)
	if err != nil {
		panic(err)
	}
*/
	// Delete All Subscriptions
	req, err = http.NewRequest("DELETE", "http://localhost:8080/api/ocloudnotifications/v1/subsriptions", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, err = server.HttpClient.Do(req)
	if err != nil {
		panic(err)
	}
	close(eventCh)

	//wg.Wait()

}

