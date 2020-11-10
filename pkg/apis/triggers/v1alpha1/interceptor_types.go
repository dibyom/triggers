package v1alpha1

import (
	"context"
	"strings"

	"google.golang.org/grpc/status"
)

type InterceptorInterface interface {
	Process(ctx context.Context, r *InterceptorRequest) *InterceptorResponse
}

type InterceptorRequest struct {
	// Body is the incoming HTTP event body
	Body []byte `json:"body,omitempty"`
	// Header are the headers for the incoming HTTP event
	Header map[string][]string `json:"header,omitempty"`
	// Extensions are extra values that are added by previous interceptors in a chain
	Extensions map[string]interface{} `json:"extensions,omitempty"`

	// InterceptorParams are the user specified params for interceptor in the Trigger
	InterceptorParams map[string]interface{}`json:"interceptor_params,omitempty"`

	Context *TriggerContext
}

type TriggerContext struct {
	// EventURL is the URL of the incoming event
	EventURL string `json:"url,omitempty"`
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

type InterceptorConfigurationSpec struct {
	// ClientConfig defines how to communicate with the interceptor service
	// Required
	ClientConfig InterceptorClientConfig `json:"clientConfig"`

	// Params declare interceptor specific fields that the user can configure per Trigger.
	Params []InterceptorParamSpec `json:"params"`
}

type InterceptorClientConfig struct {
	// `url` gives the location of the webhook, in standard URL form
	// (`scheme://host:port/path`). Exactly one of `url` or `service`
	// must be specified.
	// +optional
	URL *string `json:"url,omitempty"`

	// `service` is a reference to the service for this webhook. Either
	// `service` or `url` must be specified.
	// +optional
	Service *ServiceReference `json:"service,omitempty"`

	// `caBundle` is a PEM encoded CA bundle which will be used to validate the webhook's server certificate.
	// +optional
	CABundle []byte `json:"caBundle,omitempty"`
}

type InterceptorParamSpec struct {
	// Name is the name of the param field
	Name string `json:"name"`
	// Optional is true if the field is optional
	Optional bool `json:"optional"`
}

type ServiceReference struct {
	// `namespace` is the namespace of the service.
	// Required
	Namespace string `json:"namespace"`
	// `name` is the name of the service.
	// Required
	Name string `json:"name"`

	// `path` is an optional URL path which will be sent in any request to
	// this service.
	// +optional
	Path *string `json:"path,omitempty"`

	// If specified, the port on the service that hosting webhook.
	// Default to 443 for backward compatibility.
	// `port` should be a valid port number (1-65535, inclusive).
	// +optional
	Port *int32 `json:"port,omitempty"`
}
