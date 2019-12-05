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
	"encoding/json"
	"net/http"
	"reflect"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/interpreter/functions"
	"github.com/tektoncd/triggers/pkg/interceptors"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
	"k8s.io/client-go/kubernetes"

	triggersv1 "github.com/tektoncd/triggers/pkg/apis/triggers/v1alpha1"
)

// Interceptor implements a CEL based interceptor, that uses CEL expressions
// against the incoming body and headers to match, if the expression returns
// a true value, then the interception is "successful".
type Interceptor struct {
	KubeClientSet          kubernetes.Interface
	Logger                 *zap.SugaredLogger
	CEL                    *triggersv1.CELInterceptor
	EventListenerNamespace string
}

func NewInterceptor(cel *triggersv1.CELInterceptor, k kubernetes.Interface, ns string, l *zap.SugaredLogger) interceptors.Interceptor {
	return &Interceptor{
		Logger:                 l,
		CEL:                    cel,
		KubeClientSet:          k,
		EventListenerNamespace: ns,
	}
}

func (w *Interceptor) ExecuteTrigger(payload []byte, request *http.Request, _ *triggersv1.EventListenerTrigger, _ string) ([]byte, error) {
	mapStrDyn := decls.NewMapType(decls.String, decls.Dyn)
	env, err := cel.NewEnv(
		cel.Declarations(
			decls.NewIdent("body", mapStrDyn, nil),
			decls.NewIdent("headers", mapStrDyn, nil),
			decls.NewFunction("match",
				decls.NewInstanceOverload("match_map_string_string",
					[]*exprpb.Type{mapStrDyn, decls.String, decls.String}, decls.Bool))))
	if err != nil {
		return nil, err
	}

	var jsonMap map[string]interface{}
	err = json.Unmarshal(payload, &jsonMap)
	if err != nil {
		return nil, err
	}

	evalEnv := map[string]interface{}{"body": jsonMap, "headers": request.Header}

	out, err := evaluate(w.CEL.Expression, env, evalEnv)
	if err != nil {
		return nil, err
	}

	if out != types.True {
		return nil, err
	}

	for key, expr := range w.CEL.Values {
		val, err := evaluate(expr, env, evalEnv)
		if err != nil {
			return nil, err
		}
		payload, err = sjson.SetBytes(payload, key, val)
		if err != nil {
			return nil, err
		}

	}

	return payload, nil
}

func matchHeader(vals ...ref.Val) ref.Val {
	h, err := vals[0].ConvertToNative(reflect.TypeOf(http.Header{}))
	if err != nil {
		return types.NewErr("failed to convert to http.Header: %w", err)
	}

	key, ok := vals[1].(types.String)
	if !ok {
		return types.ValOrErr(key, "unexpected type '%v' passed to match", vals[1].Type())
	}

	val, ok := vals[2].(types.String)
	if !ok {
		return types.ValOrErr(val, "unexpected type '%v' passed to match", vals[2].Type())
	}

	return types.Bool(h.(http.Header).Get(string(key)) == string(val))

}

func evaluate(expr string, env cel.Env, data map[string]interface{}) (ref.Val, error) {
	parsed, issues := env.Parse(expr)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}

	checked, issues := env.Check(parsed)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}

	prg, err := env.Program(checked, embeddedFunctions())
	if err != nil {
		return nil, err
	}

	out, _, err := prg.Eval(data)
	return out, nil
}

func embeddedFunctions() cel.ProgramOption {
	return cel.Functions(
		&functions.Overload{
			Operator: "match",
			Function: matchHeader})

}
