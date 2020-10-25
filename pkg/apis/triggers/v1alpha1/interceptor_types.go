package v1alpha1

import (
	"context"
	"strings"

	"google.golang.org/grpc/status"
)

type InterceptorInterface interface {
	Process(ctx context.Context,  *InterceptorRequest) *InterceptorResponse
}

type InterceptorRequest struct {
	// Body is the incoming HTTP event body
	Body []byte `json:"body,omitempty"`
	// Header are the headers for the incoming HTTP event
	Header map[string][]string `json:"header,omitempty"`

	// InterceptorParams are the user specified params for the interceptor
	InterceptorParams map[string]string `json:"interceptor_ params,omitempty"`

	Context *TriggerContext
}

type TriggerContext struct {
	// EventUrl is the URL of the incoming event
	EventUrl string `json:"url,omitempty"`
	// EventID is a unique ID assigned by Triggers to each event
	EventID string `json:"event_id,omitempty"`
	// TriggerID is of the form namespace/$ns/triggers/$name
	TriggerID string `json:"trigger_id,omitempty"`
}

type InterceptorResponse struct {
	// Extensions are  additional fields that is added to the interceptor event.
	// See TEP-0022. Naming TBD.
	Extensions map[string]interface{} `json:"extensions,omitempty"`
	// Continue indicates if the EventListener should continue processing the Trigger or not
	Continue bool `json:"continue,omitempty"`
	// Status is an Error status t
	Status *status.Status `json:"status,omitempty"`
}

func ParseTriggerID(triggerID string) (namespace, name string) {
	splits := strings.Split(triggerID, "/")
	if len(splits) != 4 {
		return "", ""
	}

	return splits[1], splits[3]
}
