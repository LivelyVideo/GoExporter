package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"time"
)

type payloadEntry struct {
	Filename   string
	Binarydata []byte
	Timestamp  int
}

var fileList []payloadEntry

func main() {
	// load directory to find files to run the wrapper against.  Using the directory and filenames - generate the metadata to send to
	// server, Post the metadata in a specific way - and then the metadata is needed to be used there to re-create the file in a
	// structured manner.

	directoryPath := "/go/src/files/"

	payLoad := generatePayload(directoryPath, fileList)
	// fmt.Println("Reading payload:")
	for _, entry := range payLoad {
		call("http://172.17.0.3:8080/", "POST", entry)
		fmt.Printf("%s:  - %s\n", entry.Filename, strconv.Itoa(entry.Timestamp))
	}
}

func generatePayload(directoryPath string, payload []payloadEntry) []payloadEntry {

	var b []byte
	var found bool

	files, err := ioutil.ReadDir(directoryPath)
	if err != nil {
		log.Fatal(err)
	}
	var decgrep = findDecgrep()

	for _, file := range files {
		found = false
		var filePath = directoryPath + file.Name()

		for _, v := range fileList {
			var filename = file.Name()
			if v.Filename == filename {
				found = true
				v.Binarydata, err = exec.Command(decgrep, "-f", "4", "-s", strconv.Itoa(v.Timestamp), filePath).Output()
				if err != nil {
					fmt.Println(err.Error())
				}
				v.Timestamp = int(time.Now().UnixMilli())
				fmt.Println(v.Timestamp)

			}
		}

		if !found {
			b, err = exec.Command(decgrep, "-f", "4", filePath).Output()
			if err != nil {
				fmt.Println(err.Error())
			}
			newEntry := payloadEntry{Filename: file.Name(), Timestamp: int(time.Now().UnixMilli()), Binarydata: b}
			fmt.Println(time.Now().UnixMilli())
			fileList = append(fileList, newEntry)
		}

	}

	return fileList

}

func call(urlPath, method string, payload payloadEntry) error {

	client := &http.Client{
		Timeout: time.Second * 10,
	}

	req, err := http.NewRequest(method, urlPath, bytes.NewReader(payload.Binarydata))
	if err != nil {
		fmt.Println("Error on generating request!")
		return err
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("filename", payload.Filename)
	req.Header.Set("timestamp", strconv.Itoa(payload.Timestamp))
	rsp, _ := client.Do(req)
	if rsp.StatusCode != http.StatusOK {
		log.Printf("Request failed with response code: %d", rsp.StatusCode)
	}
	return nil
}

func findDecgrep() string {

	return "/bin/decgrep"
	// var absPath string

	// fileErr := filepath.Walk("/", func(path string, info os.FileInfo, err error) error {
	// 	// path is the absolute path.
	// 	if err != nil {
	// 		fmt.Println(err)
	// 		return err
	// 	}
	// 	if info.Name() == "decgrep" {
	// 		// if the filename is found return the absolute path of the file.
	// 		absPath = path
	// 	}

	// 	return nil
	// })
	// if fileErr != nil {
	// 	fmt.Println(fileErr)
	// }
	// // fmt.Println(absPath)
	// return absPath
}
