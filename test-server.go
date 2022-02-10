package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
)

func dataIn(w http.ResponseWriter, req *http.Request) {

	// Grab header data from request to create the right location and filename to append the incoming data.

	fmt.Printf("%s:  - %s\n", req.Header["Filename"], req.Header["Timestamp"])
	justfile := strings.Join(req.Header["Filename"], "")
	filename := "/new/" + justfile
	// fmt.Println(filename)                            //Set filename and add new directory for copy destination
	buf, err := ioutil.ReadAll(req.Body)

	if err != nil {
		log.Fatal("request", err)
	}
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
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

	// file, err := os.Create("test.bin")

	// if err!=nil {
	// 	log.Fatal("file create",err)
	// }
	http.HandleFunc("/", dataIn)

	fmt.Printf("Starting server for testing HTTP POST...\n")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}

}

// f, err := os.OpenFile("access.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
// if err != nil {
// 	log.Fatal(err)
// }
// if _, err := f.Write([]byte("appended some data\n")); err != nil {
// 	log.Fatal(err)
// }
// if err := f.Close(); err != nil {
// 	log.Fatal(err)
// }

// func writeFile() {
// 	file, err := os.Create("test.bin")
// 	defer file.Close()
// 	if err != nil {
// 		log.Fatal(err)
// 	}

// 	r := rand.New(rand.NewSource(time.Now().UnixNano()))

// 	for i := 0; i < 10; i++ {

// 		s := &payload{
// 			r.Float32(),
// 			r.Float64(),
// 			r.Uint32(),
// 		}
// 		var bin\_buf bytes.Buffer
// 		binary.Write(&bin\_buf, binary.BigEndian, s)
// 		//b :=bin\_buf.Bytes()
// 		//l := len(b)
// 		//fmt.Println(l)
// 		writeNextBytes(file, bin\_buf.Bytes())

// 	}
// }
// func writeNextBytes(file *os.File, bytes []byte) {

// 	\_, err := file.Write(bytes)

// 	if err != nil {
// 		log.Fatal(err)
// 	}

// }
