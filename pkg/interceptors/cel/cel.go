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

package cel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io/ioutil"
	"net/http"
	"reflect"

	structpb "github.com/golang/protobuf/ptypes/struct"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
	celext "github.com/google/cel-go/ext"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"k8s.io/client-go/kubernetes"

	triggersv1 "github.com/tektoncd/triggers/pkg/apis/triggers/v1alpha1"
)

var _ triggersv1.InterceptorInterface = (*Interceptor)(nil)

// Interceptor implements a CEL based interceptor that uses CEL expressions
// against the incoming body and headers to match, if the expression returns
// a true value, then the interception is "successful".
type Interceptor struct {
	KubeClientSet          kubernetes.Interface
	Logger                 *zap.SugaredLogger
	CEL                    *triggersv1.CELInterceptor
	EventListenerNamespace string
}

var (
	structType = reflect.TypeOf(&structpb.Value{})
	listType   = reflect.TypeOf(&structpb.ListValue{})
	mapType    = reflect.TypeOf(&structpb.Struct{})
)


type params = triggersv1.CELInterceptor

// NewInterceptor creates a prepopulated Interceptor.
func NewInterceptor(cel *triggersv1.CELInterceptor, k kubernetes.Interface, ns string, l *zap.SugaredLogger) *Interceptor {
	return &Interceptor{
		Logger:                 l,
		CEL:                    cel,
		KubeClientSet:          k,
		EventListenerNamespace: ns,
	}
}

// ExecuteTrigger is an implementation of the Interceptor interface.
func (w *Interceptor) ExecuteTrigger(request *http.Request) (*http.Response, error) {
	env, err := makeCelEnv(request, w.EventListenerNamespace, w.KubeClientSet)
	if err != nil {
		return nil, fmt.Errorf("error creating cel environment: %w", err)
	}

	var payload = []byte(`{}`)
	if request.Body != nil {
		defer request.Body.Close()
		payload, err = ioutil.ReadAll(request.Body)
		if err != nil {
			return nil, fmt.Errorf("error reading request body: %w", err)
		}
	}

	evalContext, err := makeEvalContext(payload, request.Header, request.URL.String())
	if err != nil {
		return nil, fmt.Errorf("error making the evaluation context: %w", err)
	}

	if w.CEL.Filter != "" {
		out, err := evaluate(w.CEL.Filter, env, evalContext)
		if err != nil {
			return nil, err
		}

		if out != types.True {
			return nil, fmt.Errorf("expression %s did not return true", w.CEL.Filter)
		}
	}

	for _, u := range w.CEL.Overlays {
		val, err := evaluate(u.Expression, env, evalContext)
		if err != nil {
			return nil, err
		}

		var raw interface{}
		var b []byte

		switch val.(type) {
		case types.String:
			raw, err = val.ConvertToNative(structType)
			if err == nil {
				b, err = json.Marshal(raw.(*structpb.Value).GetStringValue())
			}
		case types.Double, types.Int:
			raw, err = val.ConvertToNative(structType)
			if err == nil {
				b, err = json.Marshal(raw.(*structpb.Value).GetNumberValue())
			}
		case traits.Lister:
			raw, err = val.ConvertToNative(listType)
			if err == nil {
				s, err := protojson.Marshal(raw.(proto.Message))
				if err == nil {
					b = []byte(s)
				}
			}
		case traits.Mapper:
			raw, err = val.ConvertToNative(mapType)
			if err == nil {
				s, err := protojson.Marshal(raw.(proto.Message))
				if err == nil {
					b = []byte(s)
				}
			}
		case types.Bool:
			raw, err = val.ConvertToNative(structType)
			if err == nil {
				b, err = json.Marshal(raw.(*structpb.Value).GetBoolValue())
			}
		default:
			raw, err = val.ConvertToNative(reflect.TypeOf([]byte{}))
			if err == nil {
				b = raw.([]byte)
			}
		}

		if err != nil {
			return nil, fmt.Errorf("failed to convert overlay result to bytes: %w", err)
		}

		payload, err = sjson.SetRawBytes(payload, u.Key, b)
		if err != nil {
			return nil, fmt.Errorf("failed to sjson for key '%s' to '%s': %w", u.Key, val, err)
		}
	}

	return &http.Response{
		Header: request.Header,
		Body:   ioutil.NopCloser(bytes.NewBuffer(payload)),
	}, nil

}

func evaluate(expr string, env *cel.Env, data map[string]interface{}) (ref.Val, error) {
	parsed, issues := env.Parse(expr)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("failed to parse expression %#v: %s", expr, issues.Err())
	}

	checked, issues := env.Check(parsed)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("expression %#v check failed: %s", expr, issues.Err())
	}

	prg, err := env.Program(checked)
	if err != nil {
		return nil, fmt.Errorf("expression %#v failed to create a Program: %s", expr, err)
	}

	out, _, err := prg.Eval(data)
	if err != nil {
		return nil, fmt.Errorf("expression %#v failed to evaluate: %s", expr, err)
	}
	return out, nil
}

func makeCelEnv(request *http.Request, ns string, k kubernetes.Interface) (*cel.Env, error) {
	mapStrDyn := decls.NewMapType(decls.String, decls.Dyn)
	return cel.NewEnv(
		Triggers(request, ns, k),
		celext.Strings(),
		cel.Declarations(
			decls.NewVar("body", mapStrDyn),
			decls.NewVar("header", mapStrDyn),
			decls.NewVar("requestURL", decls.String),
		))
}

func makeEvalContext(body []byte, h http.Header, url string) (map[string]interface{}, error) {
	var jsonMap map[string]interface{}
	err := json.Unmarshal(body, &jsonMap)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the body as JSON: %s", err)
	}
	return map[string]interface{}{
		"body":       jsonMap,
		"header":     h,
		"requestURL": url,
	}, nil
}

func (w *Interceptor) Process(ctx context.Context, r *triggersv1.InterceptorRequest) *triggersv1.InterceptorResponse {
	b, err := json.Marshal(r.InterceptorParams)
	if err != nil {
		return &triggersv1.InterceptorResponse{
			Continue: false,
			Status:   status.New(codes.InvalidArgument, fmt.Sprintf("failed to marshal json: %v", err)),
		}
	}
	p := params{}
	if err := json.Unmarshal(b, &p); err != nil {
		// Should never happen since Unmarshall only returns err if json is invalid which we already check above
		return &triggersv1.InterceptorResponse{
			Continue: false,
			Status:   status.New(codes.InvalidArgument, fmt.Sprintf("invalid json: %v", err)),
		}
	}
	ns, _ := triggersv1.ParseTriggerID(r.Context.TriggerID)

	// The first arg is a http.Request whose only purpose is to retrieve a request scoped cache for fetching secrets
	// The cache isn't perfect since each Trigger runs in a different goroutine, and is only applicable if the
	// compareSecrets function is used.
	// TODO(): We should refactor interceptors.GetSecretToken to not use a request scoped cache and instead use a Lister
	// That should also allow use to makeCelEnv once and reuse it across requests
	env, err := makeCelEnv(nil, ns, w.KubeClientSet)
	if err != nil {
		return &triggersv1.InterceptorResponse{
			Continue: false,
			Status:   status.New(codes.Internal, fmt.Sprintf("error creating cel environment: %v", err)),
		}
	}

	var payload = []byte(`{}`)
	if r.Body != nil {
		payload = r.Body
	}

	evalContext, err := makeEvalContext(payload, r.Header, r.Context.EventURL)
	if err != nil {
		return &triggersv1.InterceptorResponse{
			Continue: false,
			Status:   status.New(codes.InvalidArgument, fmt.Sprintf("error making the evaluation context: %v", err)),
		}
	}

	if p.Filter != "" {
		out, err := evaluate(p.Filter, env, evalContext)

		if err != nil {
			return &triggersv1.InterceptorResponse{
				Continue: false,
				Status:   status.New(codes.InvalidArgument, fmt.Sprintf("error evaluating cel expression: %v", err)),
			}
		}

		if out != types.True {
			return &triggersv1.InterceptorResponse{
				Continue: false,
				Status:   status.New(codes.FailedPrecondition, fmt.Sprintf("expression %s did not return true", p.Filter)),
			}
		}
	}

	// Empty JSON body bytes.
	// We use []byte instead of map[string]interface{} to allow ovewriting keys using sjson.
	var extensions []byte
	for _, u := range p.Overlays {
		val, err := evaluate(u.Expression, env, evalContext)
		if err != nil {
			return &triggersv1.InterceptorResponse{
				Continue: false,
				Status:   status.New(codes.InvalidArgument, fmt.Sprintf("error evaluating cel expression: %v", err)),
			}
		}

		var raw interface{}
		var b []byte

		switch val.(type) {
		case types.String:
			raw, err = val.ConvertToNative(structType)
			if err == nil {
				b, err = json.Marshal(raw.(*structpb.Value).GetStringValue())
			}
		case types.Double, types.Int:
			raw, err = val.ConvertToNative(structType)
			if err == nil {
				b, err = json.Marshal(raw.(*structpb.Value).GetNumberValue())
			}
		case traits.Lister:
			raw, err = val.ConvertToNative(listType)
			if err == nil {
				s, err := protojson.Marshal(raw.(proto.Message))
				if err == nil {
					b = []byte(s)
				}
			}
		case traits.Mapper:
			raw, err = val.ConvertToNative(mapType)
			if err == nil {
				s, err := protojson.Marshal(raw.(proto.Message))
				if err == nil {
					b = []byte(s)
				}
			}
		case types.Bool:
			raw, err = val.ConvertToNative(structType)
			if err == nil {
				b, err = json.Marshal(raw.(*structpb.Value).GetBoolValue())
			}
		default:
			raw, err = val.ConvertToNative(reflect.TypeOf([]byte{}))
			if err == nil {
				b = raw.([]byte)
			}
		}

		if err != nil {
			return &triggersv1.InterceptorResponse{
				Continue: false,
				Status:   status.New(codes.Internal, fmt.Sprintf("failed to convert overlay result to type: %v", err)),
			}
		}

		// TODO: For backwards compatibility, we could keep this and return the body back?
		if extensions == nil {
			extensions = []byte("{}")
		}
		extensions, err = sjson.SetRawBytes(extensions, u.Key, b)

		if err != nil {
			return &triggersv1.InterceptorResponse{
				Continue: false,
				Status:   status.New(codes.Internal, fmt.Sprintf("failed to sjson for key '%s' to '%s': %v", u.Key, val, err)),
			}
		}
	}

	if extensions == nil {
		return &triggersv1.InterceptorResponse{
			Continue: true,
		}
	}

	extensionsMap := map[string]interface{}{}
	if err := json.Unmarshal(extensions, &extensionsMap); err != nil {
		return &triggersv1.InterceptorResponse{
			Continue: false,
			Status:   status.New(codes.Internal, fmt.Sprintf("failed to unmarshall extensions into map: %v", err)),
		}
	}

	return &triggersv1.InterceptorResponse{
		Continue: true,
		Extensions: extensionsMap,
	}
}

