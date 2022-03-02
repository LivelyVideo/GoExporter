package main

import (
	"bytes"
	"context"
	// "errors"
	"regexp"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"
	"strings"
	"path/filepath"

	"github.com/namsral/flag"
)

const defaultTick = 10 * time.Second

type config struct {
	directory    string
	statshosturl string
	binarytocall string
	tick         time.Duration
}
type fileData struct {
	fileInfo os.FileInfo
	path string
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
	confDir := getenv("CONF_DIR","conf/exporter.conf")
	flags.String(flag.DefaultConfigFlagname, confDir, "Path to config file")

	var (
		directory    = flags.String("dir", "binlogs", "Directory to read log files from")
		tick         = flags.Duration("tick", defaultTick, "Ticking interval")
		statshosturl = flags.String("url", "http://stats-exporter-server.default/", "Url to use for posts to stats host")
		binarytocall = flags.String("bin", "decgrep", "Command to call to read binary log")
	)

	if err := flags.Parse(args[1:]); err != nil {
		fmt.Println("Error1:")
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
				fmt.Println("Error4:")
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

func createFileList (dir string) ([]fileData, error) {

	var fileList []fileData
	
	libRegEx, e := regexp.Compile("^.*\\.bin\\.log$")
	if e != nil {
	    log.Fatal(e)
		return nil,e
	}

	e = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
	    if err == nil && libRegEx.MatchString(info.Name()) {
			foundFile := fileData{fileInfo: info, path: path}
			fileList = append(fileList, foundFile)
	    }
	    return nil
	})
	if e != nil {
	    log.Fatal(e)
		return nil,e
	}
	return fileList, nil
}
// Cretes payloads for all files in a struct slice.
func generatePayload(c *config, payload []payloadEntry) error {
	var b []byte
	var found bool


	files, err := createFileList(strings.Trim(c.directory, "\""))
	if err != nil {
    	fmt.Println(err)
		fmt.Println("Error building filelist!")
		return err
	}

	
	fmt.Println("Checking files now: ")
	for _, file := range files {
		found = false
		var filePath = file.path
		fmt.Println("File path: " + filePath)
		for i, _ := range fileList {
			if fileList[i].Filename == filePath {
				found = true
				fileList[i].Binarydata, err = exec.Command(strings.Trim(c.binarytocall, "\""), "-f", "4", "-s", strconv.Itoa(fileList[i].Timestamp), filePath).Output() //adding timestamp to call, with flag -s
				if err != nil {
					fmt.Println("Error2:")
					fmt.Println(err.Error())
					return err
				}
				fileList[i].Timestamp = int(time.Now().UnixMilli())
			}
		}

		if !found {
			b, err = exec.Command(strings.Trim(c.binarytocall, "\""), "-f", "4", filePath).Output()
			if err != nil {
				fmt.Println("Error3:")
				fmt.Println(err.Error())
				return err
			}

			newEntry := payloadEntry{Filename: strings.TrimLeft(file.path,c.directory), Timestamp: int(time.Now().UnixMilli()), Binarydata: b}
			fileList = append(fileList, newEntry)
		}

	}
	return nil

}

func call(urlPath, method string, payload payloadEntry) error {

    client := &http.Client{
        Timeout: time.Second * 10,
    }
	url := strings.Trim(urlPath, "\"")

    req, err := http.NewRequest(method, url,  bytes.NewReader(payload.Binarydata))

	// req.Close = true
    if err != nil {
		fmt.Println("Error5")
        return fmt.Errorf("Got error %s", err.Error())
    }
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("filename", payload.Filename)
	req.Header.Set("timestamp", strconv.Itoa(payload.Timestamp))
    response, err := client.Do(req)
    if err != nil {
		fmt.Println("Error6")
        return fmt.Errorf("Got error %s", err.Error())
    }
    defer response.Body.Close()
	
    return nil

}


func getenv(key, fallback string) string {
    value := os.Getenv(key)
    if len(value) == 0 {
        return fallback
    }
    return value
}
