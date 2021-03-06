package main

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

const file_mode = 0644

func dataIn(w http.ResponseWriter, req *http.Request) {
	if req.Method == "POST" {
		// Grab header data from request to create the right location and filename to append the incoming data.
		// TODO: handle missing fields, and check if the output directory exists
		if len(req.Header["Filename"]) == 0 || len(req.Header["Timestamp"]) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Println("Missing header!")
			fmt.Println("Filename: ", req.Header["Filename"])
			fmt.Println("Timestamp: ", req.Header["Timestamp"])
			return
		}
		fmt.Printf("%s:  - %s\n", req.Header["Filename"], req.Header["Timestamp"])
		justfile := strings.Join(req.Header["Filename"], "")
		directory := getenv("OUTPUT_DIRECTORY", "/received")

		if !strings.HasSuffix(directory, "/") {
			directory = directory + "/"
		}

		buf, err := ioutil.ReadAll(req.Body)

		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			log.Fatal("Req reading data: ", err)
		}
		// func Split(s, sep string) []string

		filename, err := buildFileName(directory, justfile)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Println("Error with build name!")
			fmt.Println(err)
		} else {
			fmt.Println("Open file: " + filename)
			file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, file_mode)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Println(err)
			}

			_, err = file.Write(buf)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				log.Fatal(err)
			}
			if err := file.Close(); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				log.Fatal(err)
			}
		}

	} else {
		w.WriteHeader(http.StatusOK)

		fmt.Println("Get request from: ", req.RemoteAddr)
	}
}

// Responsible for creating directories that are needed, and handles if there are filenames of the same name, but from different pods
// A bit hacky - just grabs the directory under the 'pods' directory - and uses that to make the podname.  Since the directories under pods are the pod names
func buildFileName(directory string, filename string) (string, error) {

	stringArray := strings.Split(filename, "/")
	lookPod := false
	podName := ""

	for i, substring := range stringArray {
		if lookPod {
			podName = stringArray[i]
			lookPod = false
		}
		if substring == "pods" {
			lookPod = true
		}
	}

	path := directory + "/" + podName
	fmt.Println("Directory to build:  " + path)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(path, os.ModePerm)
		if err != nil {
			log.Println(err)
			return path, err
		}
	}

	basefile := directory + "/" + podName + "/" + stringArray[len(stringArray)-1]
	// basefile = podName + "-" + basefile

	// TODO: pull pod name from filename and add to base file

	return basefile, nil

}

func main() {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGHUP)

	defer func() {
		signal.Stop(signalChan)
		cancel()
	}()

	go func() {
		for {
			select {
			case s := <-signalChan:
				switch s {
				case os.Interrupt:
					cancel()
					os.Exit(1)
				}
			case <-ctx.Done():
				log.Printf("Done.")
				os.Exit(1)
			}
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/", dataIn)
	log.Fatalln(http.ListenAndServe(":"+getenv("SERVER_PORT", "80"), mux))

}

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}
	return value
}
