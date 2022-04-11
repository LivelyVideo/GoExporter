#!/bin/bash


echo "Reporting logs on server"
echo " "
NUMBER_OF_LOGS=$(ls $OUTPUT_DIRECTORY | wc -l)
echo $(date +%s)
echo " "
for i in $(ls $OUTPUT_DIRECTORY)
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