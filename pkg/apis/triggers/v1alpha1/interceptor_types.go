package v1alpha1

import (
	"google.golang.org/grpc/status"
)

type InterceptorRequest struct {
	// Body is the incoming HTTP event body
	Body []byte `json:"body,omitempty"`
	// Header are the headers for the incoming HTTP event
	Header map[string][]string `json:"header,omitempty"`
	// EventUrl is the URL of the incoming event
	EventURL string `json:"event_url,omitempty"`
	// InterceptorParams are the user specified params for the interceptor
	InterceptorParams map[string]interface{} `json:"interceptor_ params,omitempty"`
	// EventID is a unique ID assigned by Triggers to each event
	EventID string `json:"event_id,omitempty"`
	// TriggerID is of the form namespace/$ns/triggers/$name
	TriggerID string `json:"trigger_id,omitempty"`
	// TriggerNamespace is the namespace of the Trigger
	TriggerNamespace string `json:"trigger_namespace,omitempty`
}

type InterceptorResponse struct {
	// Extensions are  additional fields that is added to the interceptor event.
	// See TEP-0022. Naming TBD.
	Extensions map[string]interface{} `json:"extensions,omitempty"`
	Continue   bool                   `json:"continue,omitempty"`
	Status     *status.Status         `json:"status"`
}

type InterceptorInterface interface {
	Process(req *InterceptorRequest) *InterceptorResponse
}
