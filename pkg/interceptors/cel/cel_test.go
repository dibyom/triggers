package cel

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"reflect"
	"testing"

	"github.com/tektoncd/pipeline/pkg/logging"
	triggersv1 "github.com/tektoncd/triggers/pkg/apis/triggers/v1alpha1"
	fakekubeclient "knative.dev/pkg/injection/clients/kubeclient/fake"
	rtesting "knative.dev/pkg/reconciler/testing"
)

// Allow configuration via a config map

func TestInterceptor_ExecuteTrigger_Signature(t *testing.T) {
	type args struct {
		payload []byte
	}
	tests := []struct {
		name    string
		CEL     *triggersv1.CELInterceptor
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name: "simple body check with matching body",
			CEL: &triggersv1.CELInterceptor{
				Expression: "body.value == 'testing'",
			},
			args: args{
				payload: []byte(`{"value":"testing"}`),
			},
			want:    []byte(`{"value":"testing"}`),
			wantErr: false,
		},
		{
			name: "simple body check with non-matching body",
			CEL: &triggersv1.CELInterceptor{
				Expression: "body.value == 'test'",
			},
			args: args{
				payload: []byte(`{"value":"testing"}`),
			},
			wantErr: false,
		},
		{
			name: "simple header check with matching header",
			CEL: &triggersv1.CELInterceptor{
				Expression: "headers['X-Test'][0] == 'test-value'",
			},
			args: args{
				payload: []byte(`{}`),
			},
			want:    []byte(`{}`),
			wantErr: false,
		},
		{
			name: "simple header check with non matching header",
			CEL: &triggersv1.CELInterceptor{
				Expression: "headers['X-Test'][0] == 'unknown'",
			},
			args: args{
				payload: []byte(`{}`),
			},
			wantErr: false,
		},
		{
			name: "simple header check with case insensitive failed match",
			CEL: &triggersv1.CELInterceptor{
				Expression: "headers.match('x-test', 'no-match')",
			},
			args: args{
				payload: []byte(`{}`),
			},
			wantErr: false,
		},
		{
			name: "simple header check with case insensitive matching",
			CEL: &triggersv1.CELInterceptor{
				Expression: "headers.match('x-test', 'test-value')",
			},
			args: args{
				payload: []byte(`{}`),
			},
			want:    []byte(`{}`),
			wantErr: false,
		},

		{
			name: "body and header check",
			CEL: &triggersv1.CELInterceptor{
				Expression: "headers['X-Test'][0] == 'test-value' && body.value == 'test'",
			},
			args: args{
				payload: []byte(`{"value":"test"}`),
			},
			want:    []byte(`{"value":"test"}`),
			wantErr: false,
		},
		{
			name: "unable to parse the expression",
			CEL: &triggersv1.CELInterceptor{
				Expression: "headers['X-Test",
			},
			args: args{
				payload: []byte(`{"value":"test"}`),
			},
			wantErr: true,
		},
		{
			name: "unable to parse the JSON body",
			CEL: &triggersv1.CELInterceptor{
				Expression: "body.value == 'test'",
			},
			args: args{
				payload: []byte(`{]`),
			},
			wantErr: true,
		},
		{
			name: "passing check populates a value",
			CEL: &triggersv1.CELInterceptor{
				Expression: "body.value == 'testing'",
				Values: map[string]string{
					"test.value": "body.value",
				},
			},
			args: args{
				payload: []byte(`{"value":"testing"}`),
			},
			want:    []byte(`{"test":{"value":"testing"},"value":"testing"}`),
			wantErr: false,
		},
	}
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
				Body: ioutil.NopCloser(bytes.NewReader(tt.args.payload)),
				Header: http.Header{
					"Content-Type": []string{"application/json"},
					"X-Test":       []string{"test-value"},
				},
			}
			got, err := w.ExecuteTrigger(tt.args.payload, request, nil, "")
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
