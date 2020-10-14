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

package gitlab

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/tektoncd/triggers/pkg/interceptors"
	"google.golang.org/grpc/status"

	triggersv1 "github.com/tektoncd/triggers/pkg/apis/triggers/v1alpha1"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"k8s.io/client-go/kubernetes"
)

var _ triggersv1.InterceptorInterface = (*Interceptor)(nil)

type Interceptor struct {
	KubeClientSet          kubernetes.Interface
	Logger                 *zap.SugaredLogger
	GitLab                 *triggersv1.GitLabInterceptor
	EventListenerNamespace string
}

type params struct {
	SecretRef  *SecretRef `json:"secretRef,omitempty"`
	EventTypes []string   `json:"eventTypes,omitempty"`
}

func NewInterceptor(gl *triggersv1.GitLabInterceptor, k kubernetes.Interface, ns string, l *zap.SugaredLogger) *Interceptor {
	return &Interceptor{
		Logger:                 l,
		GitLab:                 gl,
		KubeClientSet:          k,
		EventListenerNamespace: ns,
	}
}

func (w *Interceptor) ExecuteTrigger(request *http.Request) (*http.Response, error) {
	// Validate the secret first, if set.
	if w.GitLab.SecretRef != nil {
		header := request.Header.Get("X-GitLab-Token")
		if header == "" {
			return nil, errors.New("no X-GitLab-Token header set")
		}

		secretToken, err := interceptors.GetSecretToken(request, w.KubeClientSet, w.GitLab.SecretRef, w.EventListenerNamespace)
		if err != nil {
			return nil, err
		}

		// Make sure to use a constant time comparison here.
		if subtle.ConstantTimeCompare([]byte(header), secretToken) == 0 {
			return nil, errors.New("Invalid X-GitLab-Token")
		}
	}
	if w.GitLab.EventTypes != nil {
		actualEvent := request.Header.Get("X-GitLab-Event")
		isAllowed := false
		for _, allowedEvent := range w.GitLab.EventTypes {
			if actualEvent == allowedEvent {
				isAllowed = true
				break
			}
		}
		if !isAllowed {
			return nil, fmt.Errorf("event type %s is not allowed", actualEvent)
		}
	}

	return &http.Response{
		Header: request.Header,
		Body:   request.Body,
	}, nil
}

func (w *Interceptor) Process(r *triggersv1.InterceptorRequest) *triggersv1.InterceptorResponse {
	// HACK
	// Validate the secret first, if set.
	b, err := json.Marshal(r.InterceptorParams)
	if err != nil {
		return &triggersv1.InterceptorResponse{
			Continue: false,
			Status:   status.New(codes.InvalidArgument, fmt.Sprintf("failed to marshal json: %v", err)),
		}
	}
	var p *params
	if err := json.Unmarshal(b, p); err != nil {
		// Should never happen since Unmarshall only returns err if json is invalid which we already check above
		return &triggersv1.InterceptorResponse{
			Continue: false,
			Status:   status.New(codes.InvalidArgument, fmt.Sprintf("invalid json: %v", err)),
		}
	}

	if p.SecretRef != nil {
		header := http.Header(r.Header).Get("X-GitLab-Token")
		if header == "" {
			return status. errors.New("no X-GitLab-Token header set")
		}

		secretToken, err := interceptors.GetSecretToken(request, w.KubeClientSet, w.GitLab.SecretRef, w.EventListenerNamespace)
		if err != nil {
			return nil, err
		}

		// Make sure to use a constant time comparison here.
		if subtle.ConstantTimeCompare([]byte(header), secretToken) == 0 {
			return nil, errors.New("Invalid X-GitLab-Token")
		}
	}
	if w.GitLab.EventTypes != nil {
		actualEvent := request.Header.Get("X-GitLab-Event")
		isAllowed := false
		for _, allowedEvent := range w.GitLab.EventTypes {
			if actualEvent == allowedEvent {
				isAllowed = true
				break
			}
		}
		if !isAllowed {
			return nil, fmt.Errorf("event type %s is not allowed", actualEvent)
		}
	}

	return &http.Response{
		Header: request.Header,
		Body:   request.Body,
	}, nil

	return nil
}
