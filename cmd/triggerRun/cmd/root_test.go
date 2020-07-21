/*
Copyright 2020 The Tekton Authors

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

package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"reflect"
	"regexp"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/triggers/pkg/apis/triggers/v1alpha1"
	triggersv1 "github.com/tektoncd/triggers/pkg/apis/triggers/v1alpha1"
	v1alpha1 "github.com/tektoncd/triggers/pkg/apis/triggers/v1alpha1"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestReadTrigger(t *testing.T) {
	tri, err := readTrigger("../testdata/trigger.yaml")
	if err != nil {
		t.Fatalf("failed to read trigger:%+v", err)
	}

	want := []*v1alpha1.Trigger{{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "tekton.dev/v1alpha1",
			Kind:       "Trigger",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "trigger-run",
		},
		Spec: v1alpha1.TriggerSpec{
			Bindings: []*v1alpha1.TriggerSpecBinding{
				{Name: "triggerSpecBinding1"},
				{Name: "triggerSpecBinding2"},
			},
			Template: v1alpha1.TriggerSpecTemplate{
				Name: "triggerSpecTemplate",
			},
		},
	}}

	if diff := cmp.Diff(want, tri); diff != "" {
		t.Errorf("-want +got: %s", diff)
	}

}

func TestReadHTTP(t *testing.T) {
	req, err := readHTTP("../testdata/http.txt")
	if err != nil {
		t.Fatalf("failed to read HTTP: %v", err)
	}

	out, err := httputil.DumpRequest(req, true)
	if err != nil {
		t.Fatalf("failed to read HTTP: %v", err)
	}
	outStr := string(out)
	re := regexp.MustCompile(`\r?\n`)
	outStr = re.ReplaceAllString(outStr, "\n")

	expect := `POST /foo HTTP/1.1
Content-Length: 16
Content-Type: application/json
X-Header: testheader

{"test": "body"}`

	if diff := cmp.Diff(expect, outStr); diff != "" {
		t.Errorf("-want +got: %s", diff)
	}
}

func Test_processTriggerSpec(t *testing.T) {
	type args struct {
		t        *triggersv1.TriggerSpec
		request  *http.Request
		event    []byte
		eventID  string
		eventLog *zap.SugaredLogger
	}
	eventBody := json.RawMessage(`{"repository": {"links": {"clone": [{"href": "testurl", "name": "ssh"}, {"href": "testurl", "name": "http"}]}}, "changes": [{"ref": {"displayId": "test-branch"}}]}`)
	r, err := http.NewRequest("POST", "URL", bytes.NewReader(eventBody))
	if err != nil {
		t.Errorf("Cannot create a new request:", err)
	}

	logger, _ := zap.NewProduction()
	tests := []struct {
		name    string
		args    args
		want    []json.RawMessage
		wantErr bool
	}{
		{name: "testing-name",
			args: args{
				t: &v1alpha1.TriggerSpec{
					Bindings: []*v1alpha1.TriggerSpecBinding{
						{Name: "triggerSpecBinding1"},
						{Name: "triggerSpecBinding2"},
					},
					Template: v1alpha1.TriggerSpecTemplate{
						Name: "triggerSpecTemplate",
					},
				},
				request:  r,
				event:    eventBody,
				eventLog: logger.Sugar(),
			},
			want: []json.RawMessage{},
		},
	}

	// func processTriggerSpec(t *triggersv1.TriggerSpec, request *http.Request, event []byte, eventID string, eventLog *zap.SugaredLogger) error {

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := processTriggerSpec(tt.args.t, tt.args.request, tt.args.event, tt.args.eventID, tt.args.eventLog)
			if (err != nil) != tt.wantErr {
				t.Errorf("processTriggerSpec() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("processTriggerSpec() = %v, want %v", got, tt.want)
			}

			for r := range got {
				fmt.Print(r)
			}
		})
	}
}
