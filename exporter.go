package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/namsral/flag"
)

const defaultTick = 10 * time.Second

type config struct {
	directory    string
	statshosturl string
	binarytocall string
	tick         time.Duration
}

type payloadEntry struct {
	Filename   string
	Binarydata []byte
	Timestamp  int
}

var fileList []payloadEntry

//Initialization function - sets up the configuration fields
func (c *config) init(args []string) error {
	flags := flag.NewFlagSet(args[0], flag.ExitOnError)
	flags.String(flag.DefaultConfigFlagname, "/conf/exporter.conf", "Path to config file")

	var (
		directory    = flags.String("logs_directory", "/binary-files", "Directory to read log files from")
		tick         = flags.Duration("tick", defaultTick, "Ticking interval")
		statshosturl = flags.String("server_url", "http://stats-exporter-server:8080/", "Url to use for posts to stats host")
		binarytocall = flags.String("binary", "/bin/decgrep", "Command to call to read binary log")
	)

	if err := flags.Parse(args[1:]); err != nil {
		fmt.Println("Error:")
		fmt.Println(err)
		return err
	}

	c.tick = *tick
	c.directory = *directory
	c.statshosturl = *statshosturl
	c.binarytocall = *binarytocall

	return nil
}

func main() {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGHUP)

	c := &config{}

	defer func() {
		signal.Stop(signalChan)
		cancel()
	}()

	go func() {
		for {
			select {
			case s := <-signalChan:
				switch s {
				case syscall.SIGHUP:
					c.init(os.Args)
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

	if err := run(ctx, c, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, c *config, out io.Writer) error {
	c.init(os.Args)
	log.SetOutput(out)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.Tick(c.tick):
			err := generatePayload(c, fileList)
			if err != nil {
				fmt.Println("Error:")
				fmt.Println(err)
				return err
			}
			for _, entry := range fileList {
				if entry.Binarydata != nil {
					err = call(c.statshosturl, "POST", entry)
					if err != nil {
						fmt.Println("error: ")
						fmt.Println(err)
						return err
					}
					fmt.Printf("%s:  - %s\n", entry.Filename, strconv.Itoa(entry.Timestamp))
					entry.Binarydata = nil
				}
			}
		}
	}
}

// Cretes payloads for all files in a struct slice.
func generatePayload(c *config, payload []payloadEntry) error {

	var b []byte
	var found bool

	files, err := ioutil.ReadDir(c.directory)
	if err != nil {
		fmt.Println("Error:")
		fmt.Println(err)
		log.Fatal(err)
		return err
	}

	for _, file := range files {
		found = false
		var filePath = c.directory + file.Name()

		for i, _ := range fileList {
			var filename = file.Name()
			if fileList[i].Filename == filename {
				found = true
				fileList[i].Binarydata, err = exec.Command(c.binarytocall, "-f", "4", "-s", strconv.Itoa(fileList[i].Timestamp), filePath).Output() //adding timestamp to call, with flag -s
				if err != nil {
					fmt.Println("Error:")
					fmt.Println(err.Error())
					return err
				}
				fileList[i].Timestamp = int(time.Now().UnixMilli())
			}
		}

		if !found {
			b, err = exec.Command(c.binarytocall, "-f", "4", filePath).Output()
			if err != nil {
				fmt.Println("Error:")
				fmt.Println(err.Error())
				return err
			}
			newEntry := payloadEntry{Filename: file.Name(), Timestamp: int(time.Now().UnixMilli()), Binarydata: b}
			fileList = append(fileList, newEntry)
		}

	}
	return nil

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
		return errors.New(string(rsp.StatusCode))
	}
	return nil
}
