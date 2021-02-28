package types

//Event ...
// +kubebuilder:subresource:status
type Event struct {
	//	metav1.TypeMeta   `json:",inline"`
	//	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EventSpec   `json:"spec,omitempty"`
	Status EventStatus `json:"status,omitempty"`
}

//EventSpec ...
type EventSpec struct {
	// +kubebuilder:validation:MaxLength=15
	// +kubebuilder:validation:MinLength=1
	Name         string `json:"name,omitempty"`
	EndpointURI  string `json:"endpointuri,omitempty,string"`
	ResourceType string `json:"resourcetype,omitempty,string"`
	//nolint:staticcheck
	ResourceQualifier string `json:"resourcequalifier,omitempty,string"`
}

//EventStatus ...
type EventStatus struct {
	SubscriptionID string        `json:"subscriptionid,omitempty,string"`
	EventData      EventDataType `json:"resourcequalifier,omitempty,string"` //nolint:staticcheck
	URILocation    string        `json:"urilocation,omitempty,string"`
	EndpointURI    string        `json:"endpointuri,omitempty,string"`
}

//nolint:deadcode,varcheck,unused
var yaml = `apiVersion: "apiextensions.k8s.io/v1beta1" 
kind: "CustomResourceDefinition"
metadata:
  name: "fastpath.event.tracker.com"
spec:
  group: "event.tracker.com"
  version: "v1alpha1"
  scope: "Namespaced"
  names:
   plural: "events"
   singular: "event"
   kind: "Event"
validation:
  openAPIV3Schema:
    required: ["spec"]
    properties:
    spec:
      required: ["name","resourcetype","resourcequalifier"]
    properties:
      name:
        type: "string"
        minimum: 1
      resourcetype:
        type: "string"
        minimum: 1
      resourcequalifier:
        type: "string"
        minimum: 1
`
