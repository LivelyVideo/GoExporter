package main

import (
	"bytes"
	"context"
	// "errors"
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
	"errors"
	"encoding/base64"


	"github.com/namsral/flag"
)

const defaultTick = 10 * time.Second

const file_mode = 0644

type config struct {
	directory      	 string
	statshosturl   	 string
	binarytocall   	 string
	includePattern   string
	exclusionPattern string
	exDirectories    string
	tick             time.Duration
}
type fileData struct {
	fileInfo os.FileInfo
	path     string
}

type payloadEntry struct {
	Filename   string
	Binarydata []byte
	Timestamp  int
	Sent       bool
}

var fileList []payloadEntry

//Initialization function - sets up the configuration fields
func (c *config) init(args []string) error {
	flags := flag.NewFlagSet(args[0], flag.ExitOnError)
	confDir := getenv("CONF_DIR", "conf/exporter.conf")
	flags.String(flag.DefaultConfigFlagname, confDir, "Path to config file")
// Config file, flags to be used for the cli as well
	var (
		directory      	 = flags.String("dir", "binlogs", "Directory to read log files from")
		tick           	 = flags.Duration("tick", defaultTick, "Ticking interval")
		statshosturl   	 = flags.String("url", "http://stats-exporter-server.default/", "Url to use for posts to stats host")
		binarytocall   	 = flags.String("bin", "decgrep -f 5", "Executable binary, and flags, to use to read log files")
		includePattern 	 = flags.String("incl", "^.*\\.bin\\.log$", "Search pattern for binary log files")
		exclusionPattern = flags.String("excstring", "^$", "Pattern to exclude files")
		exDirectories    = flags.String("exds", "", "Slice of directories to exlude from log file search - comma delineated")
	)

	if err := flags.Parse(args[1:]); err != nil {
		fmt.Println("Error1:")
		fmt.Println(err)
		return err
	}

	c.tick             = *tick
	c.directory        = *directory
	c.statshosturl     = *statshosturl
	c.binarytocall     = *binarytocall
	c.includePattern   = *includePattern
	c.exclusionPattern = *exclusionPattern
	c.exDirectories    = *exDirectories

	return nil
}

func main() {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGHUP)

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
	c.init(os.Args)
	log.SetOutput(out)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.Tick(c.tick):
			err := generatePayload(c)
			if err != nil {
				fmt.Println("Error4:")
				fmt.Println(err)
				return err
			}


			for _, entry := range fileList {
				if (entry.Binarydata != nil) &&  (entry.Sent == false) {
					err = call(c.statshosturl, "POST", entry)
					if err != nil {
						fmt.Println("error: ")
						fmt.Println(err)
						return err
					}
					entry.Sent = true
					binaryOutput := base64.StdEncoding.EncodeToString(entry.Binarydata)
					fmt.Printf("%s:  - %s -   Binarydata: %s\n", entry.Filename, strconv.Itoa(entry.Timestamp), binaryOutput)
					// entry.Binarydata = nil
				}
			}

			newList,err := groomList(c.directory, fileList)
			if err != nil {
				fmt.Println("Error during list grooming!")
				log.Fatal(err)
			}else{
				fileList = newList
			}

		}
	}

}

// Function added to remove files from the source log file list.  
func groomList(dir string, fileList []payloadEntry)([]payloadEntry, error){

	var groomedList []payloadEntry
	fmt.Println("Grooming file list.....")
	for _,file := range fileList {
		found := false
		filename := strings.Trim(dir, "\"") + "/" + file.Filename
		// filename, err := buildFileName(dir,file.Filename)
		_, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, file_mode)
		if err != nil {
			fmt.Printf("File can't be opened! Dropping %s from file list", filename)
			fmt.Println(err)
		}else{
			fmt.Printf("File found: %s\n",filename )
			found = true
		}

		if found {
			groomedList = append(groomedList, file)
		}
	}
	return groomedList,nil
}
// TODO - refactor create file list + generate payload.  In order to avoid grooming requirements, and if the payload has been sent or not if possible

// Function that generates the filelist - this slice is used to keep track of the source logs(filename), their current []byte payloads, the timestamp for their last decgrep checks,
//  As well as whether the current payload has been sent yet or not
func createFileList(c *config, dir string) ([]fileData, error) {

	var fileList []fileData

	inclPattern := strings.Trim(c.includePattern, "\"")
	libRegEx, e := regexp.Compile(inclPattern)
	if e != nil {
		log.Fatal(e)
		return nil, e
	}
	exRegEx, e := regexp.Compile(c.exclusionPattern)
	if e != nil {
		log.Fatal(e)
		return nil, e
	}

	exclusionDirectories := strings.Split(c.exDirectories, ",")
	//Check if no exclusion directory case (1 item in a array of blank string), set to nil
	if len(exclusionDirectories) == 1 && exclusionDirectories[0] == "" {
		exclusionDirectories = nil
	}
	e = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && libRegEx.MatchString(info.Name()) &&  !(exRegEx.MatchString(info.Name())) {
			if len(exclusionDirectories) > 0 {
				for _, exDirectory := range exclusionDirectories {
					fmt.Println("Exclusion directory " + exDirectory)
					if info.IsDir() && info.Name() == exDirectory {
						fmt.Println("Skipping " + info.Name())
						return filepath.SkipDir
					}
				}
			}
			foundFile := fileData{fileInfo: info, path: path}
			// fmt.Println("Found file " + info.Name())
			fileList = append(fileList, foundFile)
		}
		return nil
	})
	if e != nil {
		log.Fatal(e)
		return nil, e
	}
	return fileList, nil
}

// Grabs payloads for all files in the filelist struct slice, and adds a current payload and timestamp to it. Creates the command lne and flags for decgrep, then calls decgrep and uses the output for the payload
func generatePayload(c *config) error {
	var b []byte
	var found bool

	files, err := createFileList(c, strings.Trim(c.directory, "\""))
	if err != nil {
		fmt.Println(err)
		fmt.Println("Error building filelist!")
		return err
	}

	binary := strings.Trim(c.binarytocall,"\"")
	commandLine := strings.Fields(binary)


	command, commandLine := commandLine[0], commandLine[1:]

	command = strings.Trim(command, "\"")
	var commandLineNew []string = nil

	for _, file := range files {
		found = false
		var filePath = strings.TrimLeft(file.path, c.directory+"/")

		for i, _ := range fileList {
			if fileList[i].Filename == filePath {
				found = true
				now := int(time.Now().UnixMilli())
				// Generating the command line for decgrep with timestamp (string slice) May need use of -d for duration?
				commandLineNew = append(commandLine, "-s")
				commandLineNew = append(commandLineNew, strconv.Itoa(fileList[i].Timestamp))

				commandLineNew = append(commandLineNew, file.path)
				
				fmt.Println("File found in file list - printing command line....")
				for _,segment := range commandLineNew {
					fmt.Println(segment)
				}
				// Checking might need to avoid re-calling decgrep on file that hasn't had payload sent yet.  
				if fileList[i].Sent {
					fmt.Printf("Found file that is being used for decgrep and has a sent flag! %s  %s", fileList[i].Filename, strconv.Itoa(fileList[i].Timestamp))
				}
				b, err = exec.Command(command, commandLineNew...).Output() //adding timestamp to call, with flag -s
				if err != nil {
					fmt.Println("Error2:")
					fmt.Println(err.Error())
					return err
				}

				fileList[i].Timestamp = now
				if b != nil {
					if  bytes.Compare(fileList[i].Binarydata , b) == 0 {
						fmt.Println("Duplicate binary data....file: " + fileList[i].Filename + " timestamp: " + strconv.Itoa(fileList[i].Timestamp))
						fileList[i].Sent = true
					} else {
						fileList[i].Sent = false
						fileList[i].Binarydata = b
					}
					
				} else {
					fmt.Println("File found and binarydata was nil")
					fmt.Println(file.path)
				}

				
			}
		}
		// File that 
		if !found {
			timestamp := int(time.Now().UnixMilli())

			//Create command  without duration - initial call
			// Setup better way to create command - also must be configurable, change flags - etc.

			commandLineNew = append(commandLine, "-s")
			commandLineNew = append(commandLineNew, strconv.Itoa(timestamp))
			commandLineNew = append(commandLineNew, file.path)

			fmt.Println("File not found in file list, first decgrep - printing command line....")
			fmt.Println(file.path)
			for _,segment := range commandLineNew {
				fmt.Println(segment)
			}

			b, err = exec.Command(command, commandLineNew...).Output()
			if err != nil {
				fmt.Println("Error3:")
				fmt.Println(err.Error())
				return err
			}

			newEntry := payloadEntry{Filename: strings.TrimLeft(file.path, c.directory), Timestamp: timestamp, Binarydata: b, Sent: false}
			fileList = append(fileList, newEntry)
		}

	}

	return nil

}

//Does the http request, with the payload to post the data
func call(urlPath, method string, payload payloadEntry) error {

	client := &http.Client{
		Timeout: time.Second * 10,
	}
	url := strings.Trim(urlPath, "\"")
	if payload.Sent == false {
		req, err := http.NewRequest(method, url, bytes.NewReader(payload.Binarydata))
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
	}else{
		// Using a sent bool to tell if the payload was sent.  To avoid duplicates
		fmt.Println("Payload already sent - " + payload.Filename)
	}
	return nil

}

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}
	return value
}

// Generates the filename based on the directory used in the config
// Pulls the podname in order to deal with multiple files with same name on single 
// host. TODO: Might need more to hit log files that are NOT transcode and keep the pod names
func buildFileName(directory string, filename string) (string, error) {

	stringArray := strings.Split(filename,"/")
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
	// fmt.Println("Directory to build:  " + path)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(path, os.ModePerm)
		if err != nil {
			log.Println(err)
			return path,err
		}
	}

	basefile := directory + "/" + podName + "/" + stringArray[len(stringArray)-1]

	return basefile,nil

}