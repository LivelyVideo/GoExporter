# GoExporter

## For Stats Pipeline...exporter for binary logs. That http server is where the binary logs will be aggregated.  A basic binary log shipper (binary filebeat - )


on the host,  agent should 
- continuously make a call to a custom exe / script which then returns a chunk of binary data, a target filename & opaque string. 
- The agent then POSTs the chunk of data over HTTP to the stats agent with the filename as a header or query parameter and stores the opaque data in memory and on file.
- In the next iteration it should pass the opaque data from previous iteration to the plugin and repeat the cycle.

(read data into function as filename to be applying decgrep script to, read output from there. Use a string format to combine `/bin/decgrep -f 4` with filename.  Filename would be used for metadata as mentioned above)


- on the sever side the HTTP handler should append to the specified file the binary data