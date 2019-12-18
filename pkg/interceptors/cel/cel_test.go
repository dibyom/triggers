package cel

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"reflect"
	"testing"

	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/tektoncd/pipeline/pkg/logging"
	triggersv1 "github.com/tektoncd/triggers/pkg/apis/triggers/v1alpha1"
	fakekubeclient "knative.dev/pkg/client/injection/kube/client/fake"
	rtesting "knative.dev/pkg/reconciler/testing"
)

// Allow configuration via a config map

func TestInterceptor_ExecuteTrigger(t *testing.T) {
	tests := []struct {
		name    string
		CEL     *triggersv1.CELInterceptor
		payload []byte
		want    []byte
		wantErr bool
	}{{
	//	name: "simple body check with matching body",
	//	CEL: &triggersv1.CELInterceptor{
	//		Expression: "body.value == 'testing'",
	//	},
	//	payload: []byte(`{"value":"testing"}`),
	//	want:    []byte(`{"value":"testing"}`),
	//}, {
		name: "simple body check with non-matching body",
		CEL: &triggersv1.CELInterceptor{
			Expression: "body.value == 'test'",
		},
		payload: []byte(`{"value":"testing"}`),
	}, {
		name: "simple header check with matching header",
		CEL: &triggersv1.CELInterceptor{
			Expression: "headers['X-Test'][0] == 'test-value'",
		},
		payload: []byte(`{}`),
		want:    []byte(`{}`),
	}, {
		name: "simple header check with non matching header",
		CEL: &triggersv1.CELInterceptor{
			Expression: "headers['X-Test'][0] == 'unknown'",
		},
		payload: []byte(`{}`),
		wantErr: false,
	}, {
		name: "overloaded header check with case insensitive failed match",
		CEL: &triggersv1.CELInterceptor{
			Expression: "headers.match('x-test', 'no-match')",
		},
		payload: []byte(`{}`),
	}, {
		name: "overloaded header check with case insensitive matching",
		CEL: &triggersv1.CELInterceptor{
			Expression: "headers.match('x-test', 'test-value')",
		},
		payload: []byte(`{}`),
		want:    []byte(`{}`),
	}, {
		name: "body and header check",
		CEL: &triggersv1.CELInterceptor{
			Expression: "headers.match('x-test', 'test-value') && body.value == 'test'",
		},
		payload: []byte(`{"value":"test"}`),
		want:    []byte(`{"value":"test"}`),
	}, {
		name: "unable to parse the expression",
		CEL: &triggersv1.CELInterceptor{
			Expression: "headers['X-Test",
		},
		payload: []byte(`{"value":"test"}`),
		wantErr: true,
	}, {
		name: "unable to parse the JSON body",
		CEL: &triggersv1.CELInterceptor{
			Expression: "body.value == 'test'",
		},
		payload: []byte(`{]`),
		wantErr: true,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := rtesting.SetupFakeContext(t)
			logger, _ := logging.NewLogger("", "")
			kubeClient := fakekubeclient.Get(ctx)
			w := &Interceptor{
				KubeClientSet: kubeClient,
				CEL:           tt.CEL,
				Logger:        logger,
			}
			request := &http.Request{
				Body: ioutil.NopCloser(bytes.NewReader(tt.payload)),
				Header: http.Header{
					"Content-Type": []string{"application/json"},
					"X-Test":       []string{"test-value"},
				},
			}
			got, err := w.ExecuteTrigger(tt.payload, request, nil, "")
			if (err != nil) != tt.wantErr {
				t.Errorf("Interceptor.ExecuteTrigger() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Interceptor.ExecuteTrigger() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestExpressionEvaluation(t *testing.T) {
	jsonMap := map[string]interface{}{
		"value": "testing",
		"sha":   "ec26c3e57ca3a959ca5aad62de7213c562f8c821",
	}
	header := http.Header{}
	evalEnv := map[string]interface{}{"body": jsonMap, "headers": header}
	env, err := makeCelEnv()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		expr string
		want ref.Val
	}{
		{
			name: "simple body value",
			expr: "body.value",
			want: types.String("testing"),
		},
		{
			name: "truncate a long string",
			expr: "truncate(body.sha, 7)",
			want: types.String("ec26c3e"),
		},
		{
			name: "boolean body value",
			expr: "body.value == 'testing'",
			want: types.Bool(true),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evaluate(tt.expr, env, evalEnv)
			if err != nil {
				t.Errorf("evaluate() got an error %s", err)
				return
			}
			_, ok := got.(*types.Err)
			if ok {
				t.Errorf("error evaluating expression: %s", got)
				return
			}

			if !got.Equal(tt.want).(types.Bool) {
				t.Errorf("evaluate() = %s, want %s", got, tt.want)
			}
		})
	}
}
