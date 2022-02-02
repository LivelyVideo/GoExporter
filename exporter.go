
package main

import (
	"github.com/hpcloud/tail"
	"bytes"
	"fmt"
	"log"
	"net/http"
	"time"
)

func main() {
	err := call("http://172.17.0.2:8080/", "POST")
	if err != nil {
		fmt.Printf("Error occurred. Err: %s", err.Error())
	}
}

func call(urlPath, method string) error {
	client := &http.Client{
		Timeout: time.Second * 10,
	}
	t, err := tail.TailFile("/go/test", tail.Config{Follow: true})
	if err != nil {
		fmt.Println("Looks like an error!")
	}
	for line := range t.Lines {
		fmt.Println(line.Text)
		b := []byte(line.Text)

		req, err := http.NewRequest(method, urlPath, bytes.NewReader(b))
		if err != nil {
			fmt.Println("Error on post!")
			return err
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		rsp, _ := client.Do(req)
		if rsp.StatusCode != http.StatusOK {
			log.Printf("Request failed with response code: %d", rsp.StatusCode)
		}

	}
	return nil
}