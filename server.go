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
	"errors"
	"time"
	"strconv"
	"sort"
	"bytes"
	// "encoding/base64"

)

type event struct {
	binaryData   []byte
	filename   	 string
	timestamp	 int

}


var entryList []event


const file_mode = 0644

func dataIn(w http.ResponseWriter, req *http.Request) {

	var duplicate bool

	directory := getenv("OUTPUT_DIRECTORY","/received")

	if req.Method == "POST" {
		duplicate = false
		// Grab header data from request to create the right location and filename to append the incoming data.
    	// TODO: handle missing fields, and check if the output directory exists
		// fmt.Printf("%s:  - %s\n", req.Header["Filename"], req.Header["Timestamp"])
		justfile := strings.Join(req.Header["Filename"], "")
	


		if !strings.HasSuffix(directory, "/") {
			directory = directory + "/"
		}

		buf, err := ioutil.ReadAll(req.Body)

		if err != nil {
			log.Fatal("Req reading data: ", err)
		}
		timestamp, err := strconv.Atoi(strings.Join(req.Header["Timestamp"],"") )
		if err != nil {
			log.Fatal("Error generating timestamp int", err)
		}

		for _,event := range entryList {
			if (justfile == event.filename) && (timestamp == event.timestamp) {
				duplicate = true
				// fmt.Println("File found: " + event.filename)
				// if  bytes.Compare(event.binaryData , buf) == 0 {
				// 	duplicate = true
				// 	fmt.Println(req.Header["Timestamp"])
				// 	fmt.Println("Duplicate data from last receive! " + justfile)
				// 	sEnc := base64.StdEncoding.EncodeToString([]byte(buf))
				// 	fmt.Println("Base64 of request buffer:")
				// 	fmt.Println(string(sEnc))
				// 	fmt.Println("Base64 of saved data:")
				// 	hEnc := base64.StdEncoding.EncodeToString([]byte(event.binaryData))
				// 	fmt.Println(string(hEnc))		
				// } 
				// event.binaryData = buf
			}
			if  bytes.Compare(event.binaryData , buf) == 0 {
				duplicate = true
			}
		}

		if !duplicate {
			newEntry := event{filename:justfile , binaryData: buf, timestamp: timestamp }
			entryList = append(entryList, newEntry)
		}
		// if !duplicate {
		// 	filename, err := buildFileName(directory,justfile)
		// 	if err != nil {
		// 		fmt.Println("Error with build name!")
		// 		fmt.Println(err)
		// 	}else{
		// 		file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, file_mode)
		// 		if err != nil {
		// 			fmt.Println(err)
		// 		}
		// 		fmt.Printf("%s:  - %s -   Binarydata: %s\n", filename, req.Header["Timestamp"],  base64.StdEncoding.EncodeToString([]byte(buf)))
		// 		_, err = file.Write(buf)
		// 		if err != nil {
		// 			fmt.Println("Error writing file")
		// 			fmt.Println(err)
		// 			log.Fatal(err)
		// 		}
		// 		if err := file.Close(); err != nil {
		// 			fmt.Println("Error closing file: ")
		// 			fmt.Println(err)
		// 			log.Fatal(err)
		// 		}
		// 	}
		// }
		// func Split(s, sep string) []string
	if len(entryList) >= 215 {
		err := handleFiles()
		if err != nil {
			fmt.Println("Error with file handler!")
			fmt.Println(err)
			log.Fatal(err)
		}
	}
	

	} else {
		fmt.Println("Not a POST!")
	}
}

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
	// basefile = podName + "-" + basefile

	// TODO: pull pod name from filename and add to base file
	
	// if strings.Index(filename, "blue") > 0 {
	// 	basefile = "blue-" + basefile 
	// }
	// if strings.Index(filename, "green") > 0 {
	// 	basefile = "green-" + basefile
	// }

	return basefile,nil

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
	log.Fatalln(http.ListenAndServe(":"+getenv("SERVER_PORT","80"), mux))


}



func getenv(key, fallback string) string {
    value := os.Getenv(key)
    if len(value) == 0 {
        return fallback
    }
    return value
}


func handleFiles() error {

	directory := getenv("OUTPUT_DIRECTORY","/received") 
	
	sort.SliceStable(entryList, func(i, j int) bool { return entryList[i].filename < entryList[j].filename })
	sort.SliceStable(entryList, func(i, j int) bool { return entryList[i].timestamp < entryList[j].timestamp })

	// for i,entry := range entryList {
	// 	for j,secEntry := range entryList {
	// 		if i != j {
	// 			if entry == secEntry {

	// 			}


	// 		}

	uniqueList := unique(entryList)

    for _,entry := range uniqueList {
			
		filename, err := buildFileName(directory,entry.filename)
		if err != nil {
			fmt.Println("Error with build name!")
			fmt.Println(err)
			return err
		}else{
			file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, file_mode)
			if err != nil {
				fmt.Println(err)
				return err
			}
			fmt.Printf("%s:  - %s \n", filename, strconv.Itoa(entry.timestamp))
			_, err = file.Write(entry.binaryData)
			if err != nil {
				fmt.Println("Error writing file")
				fmt.Println(err)
				log.Fatal(err)
				return err
			}
			if err := file.Close(); err != nil {
				fmt.Println("Error closing file: ")
				fmt.Println(err)
				log.Fatal(err)
				return err
			}
		}
	}
	fmt.Println("Emptying list!")
	entryList = nil
				// if !duplicate {
		// 	filename, err := buildFileName(directory,justfile)
		// 	if err != nil {
		// 		fmt.Println("Error with build name!")
		// 		fmt.Println(err)
		// 	}else{
		// 		file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, file_mode)
		// 		if err != nil {
		// 			fmt.Println(err)
		// 		}
		// 		fmt.Printf("%s:  - %s -   Binarydata: %s\n", filename, req.Header["Timestamp"],  base64.StdEncoding.EncodeToString([]byte(buf)))
		// 		_, err = file.Write(buf)
		// 		if err != nil {
		// 			fmt.Println("Error writing file")
		// 			fmt.Println(err)
		// 			log.Fatal(err)
		// 		}
		// 		if err := file.Close(); err != nil {
		// 			fmt.Println("Error closing file: ")
		// 			fmt.Println(err)
		// 			log.Fatal(err)
		// 		}
		// 	}
		// 
	
	return nil
}

func delaySecond(n time.Duration) {
	time.Sleep(n * time.Second)
}
func entriesEqual(first, second event) bool {
	if first.filename == second.filename && first.timestamp == second.timestamp && (bytes.Compare(first.binaryData , second.binaryData) == 0) {
		return true
	}else{
		return false
	}
}

func unique(sample []event) []event {
	var unique []event
	var found bool
	
	for _,firstEntry := range sample {
		found = false
		for _,secondEntry := range unique {
			if firstEntry.filename == secondEntry.filename && firstEntry.timestamp == secondEntry.timestamp && (bytes.Compare(firstEntry.binaryData , secondEntry.binaryData) == 0) {
				found = true
				fmt.Printf("Already found: %s - %s",firstEntry.filename,firstEntry.timestamp)
			} 
		}
		if !found {
			unique = append(unique, firstEntry)
		}
	}
	return unique
}