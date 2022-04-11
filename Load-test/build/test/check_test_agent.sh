#!/bin/bash

# LOAD_TEST_HOST=$(<$DIRECTORY/loadtest-host)
# echo "Reporting logs on $LOAD_TEST_HOST"
# echo " "
# NUMBER_OF_LOGS=$(ls $DIRECTORY | wc -l)
# echo $(date +%s)
# echo " "
for i in $(ls $DIRECTORY/*load-test*)
do 
    declare LOG_${i}=$i
    if [[ $TYPE_OF_TEST == "SFU" ]]
    then
        declare COUNT_${i}=$(/bin/livelybingrep $i | wc -l)
    else
        declare COUNT_${i}=$(/bin/decgrep $i | wc -l)
    fi
done

for j in $(seq 1 $NUMBER_OF_LOGS)
do
    echo $LOG_${j}
    echo $COUNT_${j}
    echo " "
done

