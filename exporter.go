package main

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"time"
	"os/exec"
)

func main() {
	// load directory to find files to run the wrapper against.  Using the directory and filenames - generate the metadata to send to 
	// server, Post the metadata in a specific way - and then the metadata is needed to be used there to re-create the file in a 
	// structured manner. 
	//1 Execute custom plugin - plugin is a wrapper which 
	err := call("http://172.17.0.3:8080/", "POST")  
	if err != nil {
		fmt.Printf("Error occurred. Err: %s", err.Error())
	}
}

func call(urlPath, method string) error {

	client := &http.Client{
		Timeout: time.Second * 10,
	}

	b, err := exec.Command("/go/src/wrapper-script.sh").Output()
	if err != nil {
		fmt.Println(err.Error())
	}
	
	req, err := http.NewRequest(method, urlPath, bytes.NewReader(b))
	if err != nil {
		fmt.Println("Error on generating request!")
		return err
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("filename", "yes")
	
	rsp, _ := client.Do(req)
	if rsp.StatusCode != http.StatusOK {
		log.Printf("Request failed with response code: %d", rsp.StatusCode)
	}
	return nil
}
