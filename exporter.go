package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/namsral/flag"
)

const defaultTick = 2 * time.Second

const startProbeDelay = 2 * time.Second

const file_mode = 0644

type config struct {
	directory        string
	statshosturl     string
	binarytocall     string
	includePattern   string
	exclusionPattern string
	exDirectories    string
	tick             time.Duration
}

// type fileData struct {
// 	fileInfo os.FileInfo
// 	path     string
// }

// type payloadEntry struct {
// 	fileInfo os.FileInfo
// 	Filename   string
// 	Binarydata []byte
// 	Timestamp  int
// 	Sent       bool
// }

type logFile struct {
	path       string
	fileInfo   os.FileInfo
	Binarydata []byte
	Timestamp  int
}

// var fileList []payloadEntry

//Initialization function - sets up the configuration fields
func (c *config) init(args []string) error {
	flags := flag.NewFlagSet(args[0], flag.ExitOnError)
	confDir := getenv("CONF_DIR", "conf/exporter.conf")
	flags.String(flag.DefaultConfigFlagname, confDir, "Path to config file")
	// Config file, flags to be used for the cli as well
	var (
		directory        = flags.String("dir", "binlogs", "Directory to read log files from")
		tick             = flags.Duration("tick", defaultTick, "Ticking interval")
		statshosturl     = flags.String("url", "http://stats-exporter-server.default/", "Url to use for posts to stats host")
		binarytocall     = flags.String("bin", "decgrep -f 5", "Executable binary, and flags, to use to read log files")
		includePattern   = flags.String("incl", "^.*\\.bin\\.log$", "Search pattern for binary log files")
		exclusionPattern = flags.String("excstring", "^$", "Pattern to exclude files")
		exDirectories    = flags.String("exds", "", "Slice of directories to exlude from log file search - comma delineated")
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
	c.includePattern = *includePattern
	c.exclusionPattern = *exclusionPattern
	c.exDirectories = *exDirectories

	return nil
}

func main() {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGHUP)
	fmt.Println("App started.....")
	c := &config{}
	// In order to handle signals to stop, interrupt, etc.

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

//  Main run function deals with the timing - in order to schedule how often logs are checked and then payloads sent
func run(ctx context.Context, c *config, out io.Writer) error {

	var logFiles []logFile
	c.init(os.Args)
	log.SetOutput(out)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.Tick(c.tick):
			err := createFileList(c, &logFiles)
			if err != nil {
				fmt.Println("Error4:")
				fmt.Println(err)
				return err
			}

			// Wait until server endpoint is available
			for {
				if waitUntilEndpoint(c) {
					break
				}
				time.Sleep(startProbeDelay)
			}
			// err = generatePayload(c,&logFiles)
			// if err != nil {
			// 	fmt.Println("Error4:")
			// 	fmt.Println(err)
			// 	return err
			// }

			for i, _ := range logFiles {
				fmt.Printf("Payload and post for %s\n", logFiles[i].path)
				logFiles[i].Binarydata, logFiles[i].Timestamp, err = generatePayload(c, logFiles[i])
				fmt.Printf("After generate - Timestamp for %s : %s\n", logFiles[i].path, strconv.Itoa(logFiles[i].Timestamp))
				binaryOutput := base64.StdEncoding.EncodeToString(logFiles[i].Binarydata)
				fmt.Printf("After generate - Bdata for %s: %s\n", logFiles[i].path, binaryOutput)
				if logFiles[i].Binarydata != nil {
					fmt.Println("Calling....")
					err = call(c.statshosturl, "POST", logFiles[i], c.directory)
					if err != nil {
						fmt.Println("error: ")
						fmt.Println(err)
						return err
					}
					// binaryOutput := base64.StdEncoding.EncodeToString(logFiles[i].Binarydata)
					fmt.Printf("After call: %s:  - %s \n", logFiles[i].fileInfo.Name(), strconv.Itoa(logFiles[i].Timestamp))
					// entry.Binarydata = nil
				}
			}

			err = removeFiles(c, &logFiles)
			if err != nil {
				fmt.Println("Error during list grooming!")
			}

		}
	}

}

// Searches for log files based on exclusion directories, exclusion patterns and inclusion pattern.  It updates the struct array that is passed in by reference.
// Only returns an error/or lack of one - but updates the file structs for the list of logfiles that will be used for other parts of the exporter.
func createFileList(c *config, fileList *[]logFile) error {

	// var fileList []logFile
	var exists bool
	fmt.Println("Creating file list....")
	inclPattern := strings.Trim(c.includePattern, "\"")
	libRegEx, e := regexp.Compile(inclPattern)
	if e != nil {
		log.Fatal(e)
		return e
	}
	exRegEx, e := regexp.Compile(c.exclusionPattern)
	if e != nil {
		log.Fatal(e)
		return e
	}

	exclusionDirectories := strings.Split(c.exDirectories, ",")
	//Check if no exclusion directory case (1 item in a array of blank string), set to nil
	if len(exclusionDirectories) == 1 && exclusionDirectories[0] == "" {
		exclusionDirectories = nil
	}
	e = filepath.Walk(strings.Trim(c.directory, "\""), func(path string, info os.FileInfo, err error) error {
		if err == nil && libRegEx.MatchString(info.Name()) && !(exRegEx.MatchString(info.Name())) {
			if len(exclusionDirectories) > 0 {
				for _, exDirectory := range exclusionDirectories {
					fmt.Println("Exclusion directory " + exDirectory)
					if info.IsDir() && info.Name() == exDirectory {
						fmt.Println("Skipping " + info.Name())
						return filepath.SkipDir
					}
				}
			}
			exists = false
			fmt.Println(path)
			for _, currentFile := range *fileList {
				if currentFile.path == path {
					fmt.Printf("%s already exists in the list!\n", path)
					exists = true
				}
			}
			if !exists {
				fmt.Printf("Adding %s to the list\n", path)
				foundFile := logFile{fileInfo: info, path: path, Timestamp: 0, Binarydata: nil}
				// fmt.Println("Found file " + info.Name())
				*fileList = append(*fileList, foundFile)
			}
		}
		return nil
	})
	if e != nil {
		log.Fatal(e)
		return e
	}

	return nil
}

// Grabs payloads for all files in the filelist struct slice, and adds a current payload and timestamp to it. Creates the command lne and flags for decgrep,
// then calls decgrep and uses the output for the payload
func generatePayload(c *config, file logFile) ([]byte, int, error) {

	fmt.Printf("Generate payload for %s\n", file.path)
	binary := strings.Trim(c.binarytocall, "\"")
	commandLine := strings.Fields(binary)
	command, commandLine := commandLine[0], commandLine[1:]

	command = strings.Trim(command, "\"")
	var commandLineNew []string = nil

	// for _, file := range *fileList {

	// var filePath = strings.TrimLeft(file.path, c.directory+"/")
	// At least the second time the file is being read.  Previous timestamp is used to issue the command with -s
	if file.Timestamp != 0 {
		// Generating the command line for decgrep with timestamp (string slice) May need use of -d for duration?
		commandLineNew = append(commandLine, "-s")
		commandLineNew = append(commandLineNew, strconv.Itoa(file.Timestamp))
		commandLineNew = append(commandLineNew, file.path)
		fmt.Printf("File %s has previous timestamp %s\n", file.path, strconv.Itoa(file.Timestamp))
		// Checking might need to avoid re-calling decgrep on file that hasn't had payload sent yet.
	} else {
		commandLineNew = append(commandLine, file.path)
		fmt.Printf("File %s has no previous timestamp - %s\n", file.path, strconv.Itoa(file.Timestamp))

	}

	var err error
	// Calls decgrep and receives the output as a []byte
	b, err := exec.Command(command, commandLineNew...).Output()
	if err != nil {
		fmt.Println("Error2:")
		fmt.Println(err.Error())
		return nil, file.Timestamp, err
	}
	file.Timestamp = int(time.Now().UnixMilli())

	if bytes.Compare(file.Binarydata, b) == 0 {
		fmt.Printf("%s providing duplicate binarydata from last timestamp!\n", file.path)
	}
	fmt.Printf("Returning %s timestamp for %s\n", strconv.Itoa(file.Timestamp), file.path)
	return b, file.Timestamp, nil

}

//Does the http request, with the payload to post the data. Sends filename and timestamp as headers
func call(urlPath, method string, payload logFile, directory string) error {

	fmt.Printf("Posting data from file %s\n", payload.path)

	client := &http.Client{
		Timeout: time.Second * 10,
	}
	url := strings.Trim(urlPath, "\"")
	filepath := strings.TrimLeft(payload.path, directory+"/")

	req, err := http.NewRequest(method, url, bytes.NewReader(payload.Binarydata))
	// req.Close = true
	if err != nil {
		fmt.Println("Error5")
		return fmt.Errorf("Got error %s", err.Error())
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("filename", filepath)
	req.Header.Set("timestamp", strconv.Itoa(payload.Timestamp))
	response, err := client.Do(req)
	if err != nil {
		fmt.Println("Error6")
		return fmt.Errorf("Got error %s", err.Error())
	}
	defer response.Body.Close()

	return nil

}

// Helper function to find the environment variable, or a default when it doesn't exist
func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}
	return value
}

// Removes files that no longer exist from the filelist to avoid overly long files and duplications
func removeFiles(c *config, fileList *[]logFile) error {

	var newList []logFile

	for _, logFile := range *fileList {
		_, err := os.OpenFile(logFile.path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0666)
		if os.IsNotExist(err) {
			fmt.Printf("File %s does not exist currently!\n", logFile.path)
		} else {
			newList = append(newList, logFile)
		}
	}
	*fileList = newList
	return nil
}

func waitUntilEndpoint(c *config) bool {

	url := strings.Trim(c.statshosturl, "\"")

	resp, err := http.Get(url)
	if err != nil {
		fmt.Println(err)
		return false
	}

	// Print the HTTP Status Code and Status Name
	fmt.Println("HTTP Response Status:", resp.StatusCode, http.StatusText(resp.StatusCode))

	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		fmt.Println("Endpoint up.")
		return true
	} else {
		fmt.Println("Endpoint down.")
		return false
	}

}
