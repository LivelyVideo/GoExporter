# GoExporter

## The stats exporter for the new stats-pipeline.  This service exports the binary log data (tail and append) and collects it within a singular stats server.  The binary payload is sent with metadata information that ensures the events will be placed in the correct file. The http server (server.go) is where the binary logs will be aggregated.  This service aims to be a basic binary log shipper.

## Server
The server is a basic http listener, which will accept a post with a filename and a timestamp header. The data payload is a byte array that contains the binary information from the exporter.  It will then copy the data from that post into a file named with the same name in the header, under a directory setup by the config.

These configs are currently contoler


on the host,  agent should 
- continuously make a call to a custom exe / script which then returns a chunk of binary data, a target filename & opaque string. 
- The agent then POSTs the chunk of data over HTTP to the stats agent with the filename as a header or query parameter and stores the opaque data in memory and on file.
- In the next iteration it should pass the opaque data from previous iteration to the plugin and repeat the cycle.

(read data into function as filename to be applying decgrep script to, read output from there. Use a string format to combine `/bin/decgrep -f 4` with filename.  Filename would be used for metadata as mentioned above)


- on the sever side the HTTP handler should append to the specified file the binary data


