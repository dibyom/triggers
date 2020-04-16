package main

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func main() {
	/* r = `{*/
	//"body": "'{ \"as\"}'"
	/*}A*/

	h := json.RawMessage(`{"Kind": "Task", body": "'{}'"}`)
	data := new(unstructured.Unstructured)
	if err := data.UnmarshalJSON(h); err != nil {
		fmt.Println("couldn't unmarshal json: %v", err)
	}

	fmt.Println("vim-go")
}
