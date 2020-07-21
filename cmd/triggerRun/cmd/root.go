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
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/spf13/cobra"
	"github.com/tektoncd/triggers/pkg/apis/triggers/v1alpha1"
	triggersv1 "github.com/tektoncd/triggers/pkg/apis/triggers/v1alpha1"
	V1alpha1Client "github.com/tektoncd/triggers/pkg/client/clientset/versioned/typed/triggers/v1alpha1"
	sink "github.com/tektoncd/triggers/pkg/sink"
	"github.com/tektoncd/triggers/pkg/template"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime/serializer/streaming"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	kubeconfig string
	rootCmd    = &cobra.Command{
		Use:   "triggers-run",
		Short: "This is the CLI for tekton trigger",
		Long:  "tkn-trigger will allow you",
		Run:   rootRun,
	}

	triggerFile string
	httpPath    string
)

func init() {
	rootCmd.Flags().StringVarP(&triggerFile, "triggerFile", "t", "", "Path to trigger yaml file")
	rootCmd.Flags().StringVarP(&httpPath, "httpPath", "r", "", "Path to body event")
	rootCmd.PersistentFlags().StringVarP(&kubeconfig, "kubeconfig", "k", "", "absolute path to the kubeconfig file")
}

func rootRun(cmd *cobra.Command, args []string) {
	// TODO: Not implemented
}

func trigger(w io.Writer, triggerFile, httpPath string) error {
	// Read HTTP request.
	r, err := readHTTP(httpPath)
	if err != nil {
		return fmt.Errorf("error reading HTTP file: %w", err)
	}
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("error reading HTTP body: %w", err)
	}

	// Read triggers.
	triggers, err := readTrigger(triggerFile)
	if err != nil {
		return fmt.Errorf("error reading triggers: %w", err)
	}

	client, err := GetKubeClient("kubeClient")
	if err != nil {
		fmt.Println(err)
	}
	var s sink.Sink
	for _, tri := range triggers {
		eventID := template.UID()
		eventLog := s.Logger.With(zap.String(triggersv1.EventIDLabelKey, eventID))
		output, err := processTriggerSpec(client, &tri.Spec,
			r, body, eventID, eventLog)
		if err != nil {
			return fmt.Errorf("fail to build config from the flags: %w", err)
		}
		fmt.Print(output)
	}

	return nil
}

func readTrigger(path string) ([]*v1alpha1.Trigger, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error reading trigger file: %w", err)
	}
	defer f.Close()

	var list []*v1alpha1.Trigger
	decoder := streaming.NewDecoder(f, scheme.Codecs.UniversalDecoder())
	b := new(v1alpha1.Trigger)
	for err == nil {
		_, _, err = decoder.Decode(nil, b)
		if err != nil {
			if err != io.EOF {
				return nil, fmt.Errorf("error decoding triggers: %w", err)
			}
			break
		}
		list = append(list, b)
	}
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("error decoding triggers: %w", err)
	}

	return list, nil
}

func readHTTP(path string) (*http.Request, error) {
	f, err := os.Open(path)

	if err != nil {
		return nil, fmt.Errorf("error opening file: %w", err)
	}
	defer f.Close()

	return http.ReadRequest(bufio.NewReader(f))
}

func GetKubeClient(kubeconfig string) (*V1alpha1Client.TriggersV1alpha1Client, error) {
	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("fail to build config from the flags: %w", err)
	}

	return V1alpha1Client.NewForConfig(config)
}

func processTriggerSpec(client *V1alpha1Client.TriggersV1alpha1Client, t *triggersv1.TriggerSpec, request *http.Request, event []byte, eventID string, eventLog *zap.SugaredLogger) ([]json.RawMessage, error) {
	if t == nil {
		return nil, errors.New("EventListenerTrigger not defined")
	}

	el, _ := triggersv1.ToEventListenerTrigger(*t)
	log := eventLog.With(zap.String(triggersv1.TriggerLabelKey, el.Name))
	var r sink.Sink
	finalPayload, header, err := r.ExecuteInterceptors(&el, request, event, log)
	if err != nil {
		log.Error(err)
		return nil, err
	}

	rt, err := template.ResolveTrigger(el,
		client.TriggerBindings("").Get,
		client.ClusterTriggerBindings().Get,
		client.TriggerTemplates("").Get)
	if err != nil {
		log.Error(err)
		return nil, err
	}

	params, err := template.ResolveParams(rt, finalPayload, header)
	if err != nil {
		log.Error(err)
		return nil, err
	}

	log.Infof("ResolvedParams : %+v", params)
	resources := template.ResolveResources(rt.TriggerTemplate, params)
	// for resource := range resources {
	// 	fmt.Println(resource)
	// }

	// if err := r.CreateResources("token", resources, t.Name, eventID, log); err != nil {
	// 	log.Error(err)
	// 	return err
	// }
	return resources, nil
}

// Execute runs the command.
func Execute() error {
	return rootCmd.Execute()
}
