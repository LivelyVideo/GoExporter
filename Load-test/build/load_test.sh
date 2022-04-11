#!/bin/bash
mkdir $DIRECTORY/pods/$HOSTNAME
cd $DIRECTORY/pods/$HOSTNAME
if [[ $TYPE_OF_TEST == "SFU" ]]
then
    echo "SFU test - $NUMBER_OF_STREAMS consumers at $DIRECTORY/pods/$HOSTNAME"
    /bin/livelybingrep --load-test-consumers $NUMBER_OF_STREAMS $FILENAME
else
    echo "Transcode test - $NUMBER_OF_STREAMS streams at $DIRECTORY/pods/$HOSTNAME"
    /bin/decgrep --load-test $NUMBER_OF_STREAMS $FILENAME
fi
