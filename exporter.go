package main

import (
	// "github.com/hpcloud/tail"
	"bytes"
	"fmt"
	"log"
	"net/http"
	"time"
	"os/exec"
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

	filename := "/go/test"
	b, err := exec.Command("/bin/decgrep", "-f", "4", filename).Output()
	if err != nil {
		fmt.Println(err.Error())
	}

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

	return nil
}
