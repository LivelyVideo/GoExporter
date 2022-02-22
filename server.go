package main

import (
	"context"
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

	// Grab header data from request to create the right location and filename to append the incoming data.

	fmt.Printf("%s:  - %s\n", req.Header["Filename"], req.Header["Timestamp"])
	justfile := strings.Join(req.Header["Filename"], "")
	filename := os.Getenv("OUTPUT_DIRECTORY") + justfile
	buf, err := ioutil.ReadAll(req.Body)

	if err != nil {
		log.Fatal("request", err)
	}
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, file_mode)
	if err != nil {
		fmt.Println(err)
	}

	_, err = file.Write(buf)
	if err != nil {
		log.Fatal(err)
	}
	if err := file.Close(); err != nil {
		log.Fatal(err)
	}

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
	log.Fatalln(http.ListenAndServe(":"+os.Getenv("SERVER_PORT"), mux))
}
