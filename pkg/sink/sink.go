/*
Copyright 2019 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package sink

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	triggersv1 "github.com/tektoncd/triggers/pkg/apis/triggers/v1alpha1"
	triggersclientset "github.com/tektoncd/triggers/pkg/client/clientset/versioned"
	listers "github.com/tektoncd/triggers/pkg/client/listers/triggers/v1alpha1"
	"github.com/tektoncd/triggers/pkg/interceptors"
	"github.com/tektoncd/triggers/pkg/interceptors/bitbucket"
	"github.com/tektoncd/triggers/pkg/interceptors/cel"
	"github.com/tektoncd/triggers/pkg/interceptors/github"
	"github.com/tektoncd/triggers/pkg/interceptors/gitlab"
	"github.com/tektoncd/triggers/pkg/interceptors/webhook"
	"github.com/tektoncd/triggers/pkg/resources"
	"github.com/tektoncd/triggers/pkg/template"
	"go.uber.org/zap"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	discoveryclient "k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// Sink defines the sink resource for processing incoming events for the
// EventListener.
type Sink struct {
	KubeClientSet          kubernetes.Interface
	TriggersClient         triggersclientset.Interface
	DiscoveryClient        discoveryclient.ServerResourcesInterface
	DynamicClient          dynamic.Interface
	HTTPClient             *http.Client
	EventListenerName      string
	EventListenerNamespace string
	Logger                 *zap.SugaredLogger
	Auth                   AuthOverride

	// listers index properties about resources
	EventListenerLister         listers.EventListenerLister
	TriggerLister               listers.TriggerLister
	TriggerBindingLister        listers.TriggerBindingLister
	ClusterTriggerBindingLister listers.ClusterTriggerBindingLister
	TriggerTemplateLister       listers.TriggerTemplateLister
}

// Response defines the HTTP body that the Sink responds to events with.
type Response struct {
	// EventListener is the name of the eventListener
	EventListener string `json:"eventListener"`
	// Namespace is the namespace that the eventListener is running in
	Namespace string `json:"namespace,omitempty"`
	// EventID is a uniqueID that gets assigned to each incoming request
	EventID string `json:"eventID,omitempty"`
}

// HandleEvent processes an incoming HTTP event for the event listener.
func (r Sink) HandleEvent(response http.ResponseWriter, request *http.Request) {
	el, err := r.EventListenerLister.EventListeners(r.EventListenerNamespace).Get(r.EventListenerName)
	if err != nil {
		r.Logger.Errorf("Error getting EventListener %s in Namespace %s: %s", r.EventListenerName, r.EventListenerNamespace, err)
		response.WriteHeader(http.StatusInternalServerError)
		return
	}
	event, err := ioutil.ReadAll(request.Body)
	if err != nil {
		r.Logger.Errorf("Error reading event body: %s", err)
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	eventID := template.UID()
	eventLog := r.Logger.With(zap.String(triggersv1.EventIDLabelKey, eventID))
	eventLog.Debugf("EventListener: %s in Namespace: %s handling event (EventID: %s) with payload: %s and header: %v",
		r.EventListenerName, r.EventListenerNamespace, eventID, string(event), request.Header)

	result := make(chan int, 10)
	// Execute each Trigger
	for _, t := range el.Spec.Triggers {
		go func(t triggersv1.EventListenerTrigger) {
			localRequest := request.Clone(request.Context())
			if err := r.processTrigger(&t, localRequest, event, eventID, eventLog); err != nil {
				if kerrors.IsUnauthorized(err) {
					result <- http.StatusUnauthorized
					return
				}
				if kerrors.IsForbidden(err) {
					result <- http.StatusForbidden
					return
				}
				result <- http.StatusAccepted
				return
			}
			result <- http.StatusCreated
		}(t)
	}

	//The eventlistener waits until all the trigger executions (up-to the creation of the resources) and
	//only when at least one of the execution completed successfully, it returns response code 201(Created) otherwise it returns 202 (Accepted).
	code := http.StatusAccepted
	for i := 0; i < len(el.Spec.Triggers); i++ {
		thiscode := <-result
		// current take - if someone is doing unauthorized stuff, we abort immediately;
		// unauthorized should be the final status code vs. the less than comparison
		// below around accepted vs. created
		if thiscode == http.StatusUnauthorized || thiscode == http.StatusForbidden {
			code = thiscode
			break
		}
		if thiscode < code {
			code = thiscode
		}
	}

	response.WriteHeader(code)
	response.Header().Set("Content-Type", "application/json")
	body := Response{
		EventListener: r.EventListenerName,
		Namespace:     r.EventListenerNamespace,
		EventID:       eventID,
	}
	if err := json.NewEncoder(response).Encode(body); err != nil {
		eventLog.Errorf("failed to write back sink response: %w", err)
	}
}

func (r Sink) processTrigger(t *triggersv1.EventListenerTrigger, request *http.Request, event []byte, eventID string, eventLog *zap.SugaredLogger) error {

	if t == nil {
		return errors.New("EventListenerTrigger not defined")
	}

	if t.Template == nil && t.TriggerRef != "" {
		trigger, err := r.TriggerLister.Triggers(r.EventListenerNamespace).Get(t.TriggerRef)
		if err != nil {
			r.Logger.Errorf("Error getting Trigger %s in Namespace %s: %s", t.TriggerRef, r.EventListenerNamespace, err)
			return err
		}
		trig, err := triggersv1.ToEventListenerTrigger(trigger.Spec)
		if err != nil {
			r.Logger.Errorf("Error changing Trigger to EventListenerTrigger: %s", err)
			return err
		}
		t = &trig
	}

	log := eventLog.With(zap.String(triggersv1.TriggerLabelKey, t.Name))

	finalPayload, header, iresp, err := r.ExecuteInterceptors(t, request, event, log, eventID)
	if err != nil {
		log.Error(err)
		return err
	}

	if iresp != nil {
		if !iresp.Continue {
			log.Infof("interceptor stopped trigger processing: %w", iresp.Status.Err())
			return iresp.Status.Err()
		}
	}

	rt, err := template.ResolveTrigger(*t,
		r.TriggerBindingLister.TriggerBindings(r.EventListenerNamespace).Get,
		r.ClusterTriggerBindingLister.Get,
		r.TriggerTemplateLister.TriggerTemplates(r.EventListenerNamespace).Get)
	if err != nil {
		log.Error(err)
		return err
	}
	extensions := map[string]interface{}{}
	if iresp != nil && iresp.Extensions != nil {
		extensions = iresp.Extensions
	}
	params, err := template.ResolveParams(rt, finalPayload, header, extensions)
	if err != nil {
		log.Error(err)
		return err
	}

	log.Infof("ResolvedParams : %+v", params)
	resources := template.ResolveResources(rt.TriggerTemplate, params)
	if err := r.CreateResources(t.ServiceAccountName, resources, t.Name, eventID, log); err != nil {
		log.Error(err)
		return err
	}
	return nil
}

// ExecuteInterceptor executes all interceptors for the Trigger and returns back the body, header, and InterceptorResponse to use.
// When TEP-0022 is fully implemented, this function will only return the InterceptorResponse and error.
func (r Sink) ExecuteInterceptors(t *triggersv1.EventListenerTrigger, in *http.Request, event []byte, log *zap.SugaredLogger, eventID string) ([]byte, http.Header, *triggersv1.InterceptorResponse, error) {
	if len(t.Interceptors) == 0 {
		return event, in.Header, nil, nil
	}

	// request is the request sent to the interceptors in the chain. Each interceptor can set the InterceptorParams field
	// or add to the Extensions
	request := triggersv1.InterceptorRequest{
		Body:       event,
		Header:     in.Header.Clone(),
		Extensions: map[string]interface{}{}, // Empty extensions for the first interceptor in chain
		Context: &triggersv1.TriggerContext{
			EventURL: in.URL.String(),
			EventID:  eventID,
			// t.Name might not be fully accurate until we get rid of triggers inlined within EventListener
			TriggerID: fmt.Sprintf("namespaces/%s/triggers/%s", r.EventListenerNamespace, t.Name), // TODO: t.Name might be wrong
		},
	}

	var interceptorResponse *triggersv1.InterceptorResponse
	for _, i := range t.Interceptors {
		var interceptor interceptors.Interceptor
		switch {
		case i.Webhook != nil:
			interceptor = webhook.NewInterceptor(i.Webhook, r.HTTPClient, r.EventListenerNamespace, log)
		case i.GitHub != nil:
			interceptor = github.NewInterceptor(i.GitHub, r.KubeClientSet, r.EventListenerNamespace, log)
		case i.GitLab != nil:
			interceptor = gitlab.NewInterceptor(i.GitLab, r.KubeClientSet, r.EventListenerNamespace, log)
		case i.CEL != nil:
			interceptor = cel.NewInterceptor(r.KubeClientSet, log)
		case i.Bitbucket != nil:
			interceptor = bitbucket.NewInterceptor(i.Bitbucket, r.KubeClientSet, r.EventListenerNamespace, log)
		default:
			return nil, nil, nil, fmt.Errorf("unknown interceptor type: %v", i)
		}

		// Immutable Interface check
		if interceptorInterface, ok := interceptor.(triggersv1.InterceptorInterface); ok {
			// Set per interceptor config params to the request
			request.InterceptorParams = interceptors.GetInterceptorParams(i)
			interceptorResponse = interceptorInterface.Process(context.Background(), &request)
			if !interceptorResponse.Continue {
				return nil, nil, interceptorResponse, nil
			}

			if interceptorResponse.Extensions != nil {
				// Merge any extensions and pass it on to the next request in the chain
				for k, v := range interceptorResponse.Extensions {
					request.Extensions[k] = v
				}
			}
			// Clear interceptorParams for the next interceptor in chain
			request.InterceptorParams = map[string]interface{}{}
		} else {
			// Old style interceptor (everything but CEL at the moment)
			req := &http.Request{
				Method: http.MethodPost,
				Header: request.Header,
				URL:    in.URL,
				Body:   ioutil.NopCloser(bytes.NewBuffer(request.Body)),
			}

			res, err := interceptor.ExecuteTrigger(req)
			if err != nil {
				return nil, nil, nil, err
			}

			payload, err := ioutil.ReadAll(res.Body)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("error reading webhook interceptor response body: %w", err)
			}
			defer res.Body.Close()
			// Set the next request to be the output of the last response to enable
			// request chaining.
			request.Header = res.Header.Clone()
			request.Body = payload
		}
	}
	return request.Body, request.Header, &triggersv1.InterceptorResponse{
		Continue:   true,
		Extensions: request.Extensions,
	}, nil
}

func (r Sink) CreateResources(sa string, res []json.RawMessage, triggerName, eventID string, log *zap.SugaredLogger) error {
	discoveryClient := r.DiscoveryClient
	dynamicClient := r.DynamicClient
	var err error
	if len(sa) > 0 {
		// So at start up the discovery and dynamic clients are created using the in cluster config
		// of this pod (i.e. using the credentials of the serviceaccount associated with the EventListener)

		// However, we also have a ServiceAccountName reference with each EventListenerTrigger to allow
		// for more fine grained authorization control around the resources we create below.
		discoveryClient, dynamicClient, err = r.Auth.OverrideAuthentication(sa, r.EventListenerNamespace, log, r.DiscoveryClient, r.DynamicClient)
		if err != nil {
			log.Errorf("problem cloning rest config: %#v", err)
			return err
		}
	}

	for _, rr := range res {
		if err := resources.Create(r.Logger, rr, triggerName, eventID, r.EventListenerName, r.EventListenerNamespace, discoveryClient, dynamicClient); err != nil {
			log.Errorf("problem creating obj: %#v", err)
			return err
		}
	}
	return nil
}
