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

// Simple body check with matching body
// Simple body check with non-matching body
// Simple header check with matching header
// Simple header check with non-matching header
// Simple header check with case insensitive matching
// Body and header check
// Unable to parse the expression
// Unable to parse the JSON body

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
				},
			}
			got, err := w.ExecuteTrigger(tt.args.payload, request, nil, "")
			if (err != nil) != tt.wantErr {
				t.Errorf("Interceptor.ExecuteTrigger() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Interceptor.ExecuteTrigger() = %v, want %v", got, tt.want)
			}
		})
	}
}
