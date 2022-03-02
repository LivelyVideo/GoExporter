/*
 * decgrep.c
 *
 *  Created on: May 23, 2019
 *      Author: Amir Pauker
 *
 *
 * Small utility to parse the decoder binary log
 */

#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <errno.h>
#include <string.h>
#include <fcntl.h>
#include <time.h>
#include <inttypes.h>
#include <arpa/inet.h>
#include <fnmatch.h>
#include <dirent.h>
#include <sys/stat.h>
#include <math.h>

#include "decoder_bin_log.h"

#define MAX_LABEL_LEN   200
#define MAX_DIR         2
#define MAX_FILES       3
#define MAX_TIME_ALIGN  10

#define FORMAT_TABLE    1
#define FORMAT_CSV      2
#define FORMAT_PROTOBUF 3
#define FORMAT_MSGPACK  4

#define ALT_FILE_FORMAT_PROTOBUF 1
#define ALT_FILE_FORMAT_MSGPACK  2

static char *header[][3] = {
        {"Start Time              ", "Start Time",    "The statime of the epoch (HH:MM:SS,sss)"},
        {"Epoch  ",                  "Epoch Len",     "Epoch length in milliseconds"},
        {"v_l_pts    ",              "Video PTS",     "The most recent video PTS in source buffer"},
        {"v_dts      ",              "Video DTS",     "The latet video DTS read from source buffer"},
        {"a_dts      ",              "Audio DTS",     "The latet audio DTS read from source buffer"},
        {"v_kbps ",                  "Video kbps",    "Ingress video bitrate during epoch in kbps"},
        {"a_kbps ",                  "Audio kbps",    "Ingress audio bitrate during epoch in kbps"},
        {"Sleep  ",                  "Sleep Time",    "Total sleep time in epoch in milliseconds"},

        // Audio
        {"a_drop ",                  "Drop Milli",   "The amount of audio samples that were dropped in epoch exprssed in milliseconds"},
        {"a_inj  ",                  "Inject Milli", "The amount of silence injected in epoch exprssed in milliseconds"},
        {"a_rd_c ",                  "Rd calls",     "The number of calls to read audio packet from shared memory in epoch"},
        {"a_rd   ",                  "Rd",           "The number of read audio packet from shared memory in epoch"},
        {"a_wr   ",                  "Wr calls",     "The number of calls to write audio frame to shared memory in epoch"},
        {"a_err  ",                  "Audio Errors", "Total number of audio decoding errors in epoch"},

        // Video
        {"v_mi_p ",                  "Min Pending",  "The min number of pending video frames in jitter buffer in epoch"},
        {"v_ma_p ",                  "Max Pending",  "The max number of pending video frames in jitter buffer in epoch"},
        {"v_drop ",                  "Drop Frames",  "The number of video frames that were dropped in epoch"},
        {"v_inj  ",                  "Duplicate",    "The number of video frames that were duplicated in epoch"},
        {"v_rd_c ",                  "Rd calls",     "The number of calls to read video packet from shared memory in epoch"},
        {"v_rd   ",                  "Rd",           "The number of read video packet from shared memory in epoch"},
        {"v_wr   ",                  "Wr calls",     "The number of calls to write video frame to shared memory in epoch"},
        {"v_err  ",                  "Video Errors", "Total number of video decoding errors in epoch"},
        {"v_init ",                  "Video Reinit", "Total number of video re-init decoder context in epoch"},
        {"v_w  ",                    "Video Width",  "Source video width in pixels"},
        {"v_h  ",                    "Video Height", "Source video height in pixels"},

        {"acc_p",                    "Access Param",  "Opaque data which determines if the stream is in public or private mode"},
        NULL
};

typedef struct {
    char                      *directories[MAX_DIR];   // optional list of directories to look for files to parse
    int                       num_dir;                 // number of directories
    char                      *filename;               // either a full path to a file to parse or wildcard pattern of files to parse in directories
    char                      *alt_file;               // in case the output should be formatted as protobuf or msgpack then store output into that file
    int                       alt_file_format;         // indicate the output format of alternative file one of ALT_FILE_FORMAT_XXX
    char                      *label;                  // optional lable to add to each record
    int                       format;                  // output format (csv or table)
    int                       verbose;                 // include columns description or not
    int                       dedup;                   // remove duplicates
    int                       header;                  // include headers line
    uint64_t                  start_ts;                // optional start timestamp in milliseconds
    uint64_t                  dur;                     // optional epoch length in milliseconds
    uint64_t                  dedup_last_ts;           // in case dedup is on, stores the last seen timestamp
    uint16_t                  time_align;              // interval in milliseconds for time alignment i.e. convert variable length epoch to fixed length
} decgrep_conf_t;

/******************************************************************************
 *                 Protobuf and Msgpack encode functions
 *****************************************************************************/
static inline void
enc_protobuf_uint_variant(uint64_t value, uint8_t **out)
{
    uint8_t v;

    for (;;) {
        v = (uint8_t)(value & 0x7F); // get 7 LSB at a time
        value >>= 7;
        if (value > 0) {
            v |= 0x80; // turn MSB to signal more data
            (*out)[0] = v;
            (*out)++;
        } else {
            (*out)[0] = v;
            (*out)++;
            break;
        }
    }
}

static inline void
enc_protobuf_int(int size, uint64_t index, uint64_t value, uint8_t **out)
{
    switch(size) {
    case 8:
        index = (index << 3) + 1; // wire type fixed int64
        enc_protobuf_uint_variant(index, out);
        memcpy(*out, &value, 8);
        (*out) += 8;
        break;
    case 4:
        index = (index << 3) + 5; // wire type fixed int32
        enc_protobuf_uint_variant(index, out);
        memcpy(*out, &value, 4);
        (*out) += 4;
        break;
    default:
        index <<= 3; // wire type variant int
        enc_protobuf_uint_variant(index, out);
        enc_protobuf_uint_variant(value, out);
        break;
    }
}

static inline void
enc_protobuf_string(uint64_t index, char *str, uint64_t len, uint8_t **out)
{
    index = (index << 3) + 2; // wire type string
    enc_protobuf_uint_variant(index, out);
    enc_protobuf_uint_variant(len, out);
    memcpy(*out, str, len);
    (*out) += len;
}

static inline void
enc_protobuf_record(xcode_dec_bin_log_record_t *rec, uint8_t **out)
{
    enc_protobuf_int(8, 1, rec->start_tm, out);
    enc_protobuf_int(4, 2, rec->v_latest_pts, out);
    enc_protobuf_int(4, 3, rec->v_last_dts, out);
    enc_protobuf_int(4, 4, rec->a_last_dts, out);
    enc_protobuf_int(4, 5, rec->v_ingress_bytes    , out);
    enc_protobuf_int(0, 6, rec->a_ingress_bytes, out);
    enc_protobuf_int(0, 7, rec->epoch_len, out);
    enc_protobuf_int(0, 8, rec->dec_sleep_tm, out);
    enc_protobuf_int(0, 9, rec->a_num_disc_millisec, out);
    enc_protobuf_int(0, 10, rec->a_num_inject_millisec, out);
    enc_protobuf_int(0, 11, rec->a_num_rd_calls, out);
    enc_protobuf_int(0, 12, rec->a_num_rd, out);
    enc_protobuf_int(0, 13, rec->a_num_wr, out);
    enc_protobuf_int(0, 14, rec->a_num_err, out);
    enc_protobuf_int(0, 15, rec->v_max_pending, out);
    enc_protobuf_int(0, 16, rec->v_min_pending, out);
    enc_protobuf_int(0, 17, rec->v_num_wr, out);
    enc_protobuf_int(0, 18, rec->v_num_rd_calls, out);
    enc_protobuf_int(0, 19, rec->v_num_rd, out);
    enc_protobuf_int(0, 20, rec->v_num_disc_frame, out);
    enc_protobuf_int(0, 21, rec->v_num_dup_frame, out);
    enc_protobuf_int(0, 22, rec->v_num_err, out);
    enc_protobuf_int(0, 23, rec->v_num_reinit_ctx, out);
    enc_protobuf_int(0, 24, rec->v_width, out);
    enc_protobuf_int(0, 25, rec->v_height, out);
    enc_protobuf_int(0, 26, rec->acc_param, out);
}

// stores the label and the message encoded as protobuf byte stream
// it is encoded as: <label, record length>, <record>
static inline void
enc_protobuf_write_rec_to_file(
        char *label, uint64_t label_len, xcode_dec_bin_log_record_t *rec, int fd)
{
    uint8_t          *rec_len, *end;
    uint32_t         size;
    int              rc;
    static uint8_t   buf[8 + MAX_LABEL_LEN + 3 * sizeof(xcode_dec_bin_log_record_t)];

    end = buf + 4;

    // encode the label. the label tells the
    // decoder which type of record to expect
    enc_protobuf_string(1, label, label_len, &end);

    // write the size of the label in big endian
    size = htonl(end - buf - 4);
    memcpy(buf, &size, 4);

    // place holder for record length 4 bytes
    rec_len = end;
    end += 4;

    // encode record
    enc_protobuf_record(rec, &end);

    // write the size of the record in big endian
    size = htonl(end - rec_len - 4);
    memcpy(rec_len, &size, 4);

    rc = write(fd, buf, end - buf);
    if (rc < 0) {
        fprintf(stderr, "failed to write to protobuf file. err=%s\n", strerror(errno));
        exit(1);
    }
}


// stores the label and the message encoded as msgpack byte stream
static inline void
enc_msgpack_write_rec_to_file(
        char *label, uint64_t label_len, xcode_dec_bin_log_record_t *rec, int fd)
{
    uint8_t          *p;
    int              rc;
    static uint8_t   buf[2 + MAX_LABEL_LEN + 2 + sizeof(xcode_dec_bin_log_record_t)];

    p = buf;
    if (label_len) {
    	p[0] = 0xD9;       // str up to 254 bytes long
    	p[1] = label_len;
    	p += 2;
    	memcpy(p, label, label_len);
    	p += label_len;
    }

    p[0] = 0xC4; // bin array up to 254 bytes long
    p[1] = sizeof(xcode_dec_bin_log_record_t);
    memcpy(&p[2], rec, sizeof(xcode_dec_bin_log_record_t));
    p += 2 + sizeof(xcode_dec_bin_log_record_t);


    rc = write(fd, buf, p - buf);
    if (rc < 0) {
        fprintf(stderr, "failed to write to msgpack file. err=%s\n", strerror(errno));
        exit(1);
    }
}


/*****************************************************************************/

static void
print_headers_description()
{
    int i;
    for(i = 0;*header[i];i++){
        printf("%-28s%s\n", header[i][0], header[i][2]);
    }
    printf("\n");
}


// gets a single record that spans over variable interval and
// returns an array of records that are aligned to fixed time
// intervals e.g. 1 second
int
time_alignment(
		decgrep_conf_t *conf,
		xcode_dec_bin_log_record_t *rec_in,
		xcode_dec_bin_log_record_t *rec_out,
		int out_len)
{
	xcode_dec_bin_log_record_t sample;
	int                        i;
	uint64_t                   start_tm, end_tm;

#define TIME_ALIGNMENT(metric) \
		sample.metric = rec_in->metric * conf->time_align / rec_in->epoch_len

#define TIME_ALIGNMENT_ACCURATE(metric) \
		sample.metric = round((double)rec_in->metric * (double)conf->time_align / (double)rec_in->epoch_len)

	TIME_ALIGNMENT(v_ingress_bytes);
	TIME_ALIGNMENT(a_ingress_bytes);

	TIME_ALIGNMENT_ACCURATE(a_num_disc_millisec);
	TIME_ALIGNMENT_ACCURATE(a_num_inject_millisec);
	TIME_ALIGNMENT(a_num_rd_calls);
	TIME_ALIGNMENT(a_num_rd);
	TIME_ALIGNMENT(a_num_wr);
	TIME_ALIGNMENT_ACCURATE(a_num_err);

	TIME_ALIGNMENT(v_num_wr);
	TIME_ALIGNMENT(v_num_rd_calls);
	TIME_ALIGNMENT(v_num_rd);
	TIME_ALIGNMENT_ACCURATE(v_num_disc_frame);
	TIME_ALIGNMENT_ACCURATE(v_num_dup_frame);
	TIME_ALIGNMENT_ACCURATE(v_num_err);
	TIME_ALIGNMENT_ACCURATE(v_num_reinit_ctx);

	TIME_ALIGNMENT(dec_sleep_tm);


	sample.epoch_len     = conf->time_align;
	sample.v_max_pending = rec_in->v_max_pending;
	sample.v_min_pending = rec_in->v_min_pending;
	sample.v_width       = rec_in->v_width;
	sample.v_height      = rec_in->v_height;
	sample.acc_param     = rec_in->acc_param;
	sample.v_last_dts    = rec_in->v_last_dts;
	sample.v_latest_pts  = rec_in->v_latest_pts;
	sample.a_last_dts    = rec_in->a_last_dts;

	start_tm = (rec_in->start_tm % conf->time_align) ?
			(rec_in->start_tm / conf->time_align + 1) * conf->time_align : rec_in->start_tm;
	end_tm   = rec_in->start_tm + rec_in->epoch_len;

	for (i = 0; i < out_len && start_tm < end_tm; i++, start_tm += conf->time_align) {
		rec_out[i]               = sample;
		rec_out[i].start_tm      = start_tm;
	}

	return i;
}

// table
int
format_1(int fd, int alt_file_fd, decgrep_conf_t *conf)
{
    char                        buf[sizeof(xcode_dec_bin_log_record_t) * 1000];
    int                         n, m, i, num_rec, num_tm_align_rec;
    char                        *first;
    size_t                      len;
    xcode_dec_bin_log_record_t  *rec;
    xcode_dec_bin_log_record_t  rec_tm_align[MAX_TIME_ALIGN];
    uint64_t                    end_ts;
    uint64_t                    start_ts;

    struct tm                   *gmt_tm_ptr, gmt_tm;
    time_t                      first_tm;
    int                         delta_sec, end_of_day, days, hours, minutes, seconds;

    int                         v_ingress_kbps;
    int                         a_ingress_kbps;

    char                        label_buf[MAX_LABEL_LEN];
    int                         label_len;

    if (conf->label) {
    	label_len = strlen(conf->label);
        if (label_len + 2 > sizeof(label_buf)) {
        	fprintf(stderr, "label too long %s.\n", conf->label);
        	exit(1);
        }

        strcpy(label_buf, conf->label);
        label_buf[label_len] = ' ';
        label_buf[label_len + 1] = 0;
    } else {
    	label_len = 0;
    	label_buf[0] = 0;
    }


    first = buf;
    len = sizeof(buf);

    first_tm = 0;

    if(conf->verbose)
        print_headers_description();

    // print headers row
    if (conf->header) {
    	if (label_len) {
    		printf("%*s", label_len + 1 + (int)strlen(header[0][0]), header[0][0]);
    	} else {
    		printf("%s", header[0][0]);
    	}
    	for(i = 1;*header[i];i++){
    		printf("%s", header[i][0]);
    	}
    	printf("\n");
    }

    start_ts = conf->start_ts;
    num_tm_align_rec = 1;

    while( (n = read(fd, first, len)) > 0 ){
        num_rec = n / sizeof(xcode_dec_bin_log_record_t);

        // go over all the records currently in the buffer (read from file)
        for (m = 0; m < num_rec; m++) {
        	rec = &((xcode_dec_bin_log_record_t*)buf)[m];

        	// in case remove duplicates is on and this record's timestamp
        	// is older than the most recent record then skip it
        	if (conf->dedup && rec->start_tm <= conf->dedup_last_ts) {
        		continue;
        	} else {
        		conf->dedup_last_ts = rec->start_tm;
        	}

        	// in case time alignment is set, each record in the binary log
        	// will most likely get converted into one or more sub-samples
        	// of fixed time intervals
        	if (conf->time_align) {
        		num_tm_align_rec = time_alignment(conf, rec, rec_tm_align, MAX_TIME_ALIGN);
        		rec = rec_tm_align;
        	}

        	// in case time alignment is set then it will loop over all
        	// the sub-samples, otherwise it will just loop once over the
        	// record that was read from file as is
        	for (i = 0; i < num_tm_align_rec; i++) {


        		// in order to avoid repeated calls to gmtime we calculated it once for the first record
        		// then adds the delta to it for all other records.
        		if (first_tm == 0) {
        			if (!start_ts) {
        				start_ts = rec[i].start_tm;
        			}

        			// in case duration is specified without start time, it is taken from the start of the file
        			end_ts = conf->dur? (start_ts + conf->dur) : (uint64_t)-1;

        			first_tm = rec[i].start_tm / 1000;
        			gmt_tm_ptr = gmtime(&first_tm);
        			if(!gmt_tm_ptr){
        				fprintf(stderr, "failed to convert timestamp to GMT time.\n");
        				exit(1);
        			}
        			gmt_tm = *gmt_tm_ptr;
        			// for timestamps that falls at the same day we calculate the hours, minutes and seconds
        			// based on delta from first_tm rather than calling gmtime for every timestamp since it is
        			// expensive
        			end_of_day =
        					first_tm + 3600 * 24 - gmt_tm.tm_hour * 3600 - gmt_tm.tm_min * 60 - gmt_tm.tm_sec;

        			gmt_tm.tm_year += 1900;
        		}

        		if(rec[i].start_tm < start_ts || rec[i].start_tm > end_ts){
        			continue;
        		}

        		if(rec[i].start_tm / 1000 >= end_of_day){
        			first_tm = rec[i].start_tm / 1000;
        			gmt_tm_ptr = gmtime(&first_tm);
        			if(!gmt_tm_ptr){
        				fprintf(stderr, "failed to convert timestamp to GMT time.\n");
        				exit(1);
        			}
        			gmt_tm = *gmt_tm_ptr;
        			// for timestamps that falls at the same day we calculate the hours, minutes and seconds
        			// based on delta from first_tm rather than calling gmtime for every timestamp since it is
        			// expensive
        			end_of_day =
        					first_tm + 3600 * 24 - gmt_tm.tm_hour * 3600 - gmt_tm.tm_min * 60 - gmt_tm.tm_sec;

        			gmt_tm.tm_year += 1900;
        			hours = gmt_tm.tm_hour;
        			minutes = gmt_tm.tm_min;
        			seconds = gmt_tm.tm_sec;
        		}
        		else{
        			delta_sec = rec[i].start_tm / 1000 - first_tm;
        			hours = gmt_tm.tm_hour + delta_sec / 3600;
        			delta_sec = delta_sec % 3600;
        			minutes = gmt_tm.tm_min + delta_sec / 60;
        			delta_sec = delta_sec % 60;
        			seconds = gmt_tm.tm_sec + delta_sec;

        			minutes += seconds / 60;
        			seconds = seconds % 60;
        			hours += minutes / 60;
        			minutes = minutes % 60;
        		}

        		if(rec[i].epoch_len){
        			v_ingress_kbps  = rec[i].v_ingress_bytes * 8 / rec[i].epoch_len;
        			a_ingress_kbps  = rec[i].a_ingress_bytes * 8 / rec[i].epoch_len;
        		}
        		else{
        			v_ingress_kbps = a_ingress_kbps = 0;
        		}

        		printf("%s"
        				"%04d-%02d-%02d %02d:%02d:%02d,%03d "
        				"%-7d%-11"PRIu32"%-11"PRIu32"%-11"PRIu32
        				"%-7d%-7d%-7d"
        				"%-7d%-7d%-7d%-7d%-7d%-7d"
        				"%-7d%-7d%-7d%-7d%-7d%-7d%-7d%-7d%-7d"
        				"%-5d%-5d%-5d\n",
						label_buf,
						gmt_tm.tm_year, gmt_tm.tm_mon + 1, gmt_tm.tm_mday, hours, minutes, seconds, (int)(rec[i].start_tm % 1000),
						rec[i].epoch_len, rec[i].v_latest_pts, rec[i].v_last_dts, rec[i].a_last_dts,
						v_ingress_kbps, a_ingress_kbps, rec[i].dec_sleep_tm,
						rec[i].a_num_disc_millisec, rec[i].a_num_inject_millisec, rec[i].a_num_rd_calls, rec[i].a_num_rd, rec[i].a_num_wr, rec[i].a_num_err,
						rec[i].v_min_pending, rec[i].v_max_pending, rec[i].v_num_disc_frame, rec[i].v_num_dup_frame, rec[i].v_num_rd_calls, rec[i].v_num_rd, rec[i].v_num_wr, rec[i].v_num_err, rec[i].v_num_reinit_ctx,
						rec[i].v_width, rec[i].v_height, rec[i].acc_param);

        		if (alt_file_fd > 0) {
        			switch(conf->alt_file_format) {
        			case ALT_FILE_FORMAT_PROTOBUF:
        				enc_protobuf_write_rec_to_file(label_buf, label_len, &rec[i], alt_file_fd);
        				break;
        			case ALT_FILE_FORMAT_MSGPACK:
        				enc_msgpack_write_rec_to_file(label_buf, label_len, &rec[i], alt_file_fd);
        				break;
        			}
        		}
        	}
        }

        if(n % sizeof(xcode_dec_bin_log_record_t)){
            memmove(buf, buf + sizeof(xcode_dec_bin_log_record_t) * num_rec, n % sizeof(xcode_dec_bin_log_record_t));
            first = buf + (n % sizeof(xcode_dec_bin_log_record_t));
            len = sizeof(buf) - (n % sizeof(xcode_dec_bin_log_record_t));
        }
        else{
            first = buf;
            len = sizeof(buf);
        }
    }

    return 0;
}


// csv, protobuf, msgpack
int
format_2(int fd, int alt_file_fd, decgrep_conf_t *conf)
{
    char                        buf[sizeof(xcode_dec_bin_log_record_t) * 1000];
    int                         n, m, i, num_rec, num_tm_align_rec;
    char                        *first;
    size_t                      len;
    xcode_dec_bin_log_record_t  *rec;
    xcode_dec_bin_log_record_t  rec_tm_align[MAX_TIME_ALIGN];
    int                         v_ingress_kbps;
    int                         a_ingress_kbps;
    time_t                      first_tm;
    uint64_t                    start_ts;
    uint64_t                    end_ts;

    char                        label_buf[MAX_LABEL_LEN];
    size_t                      label_len;

    if (conf->label) {
    	label_len = strlen(conf->label);
        if (label_len + 2 > sizeof(label_buf)) {
        	fprintf(stderr, "label too long %s.\n", conf->label);
        	exit(1);
        }

        strcpy(label_buf, conf->label);
        label_buf[label_len] = ',';
        label_buf[label_len + 1] = 0;
    } else {
        label_len = 0;
    	label_buf[0] = 0;
    }


    first = buf;
    len = sizeof(buf);

    first_tm = 0;
    start_ts = conf->start_ts;
    num_tm_align_rec = 1;


    while( (n = read(fd, first, len)) > 0 ){
        num_rec = n / sizeof(xcode_dec_bin_log_record_t);

        // go over all the records currently in the buffer (read from file)
        for (m = 0; m < num_rec; m++) {
        	rec = &((xcode_dec_bin_log_record_t*)buf)[m];

        	// in case remove duplicates is on and this record's timestamp
        	// is older than the most recent record then skip it
        	if (conf->dedup && rec->start_tm <= conf->dedup_last_ts) {
        		continue;
        	} else {
        		conf->dedup_last_ts = rec->start_tm;
        	}


        	// in case time alignment is set, each record in the binary log
        	// will most likely get converted into one or more sub-samples
        	// of fixed time intervals
        	if (conf->time_align) {
        		num_tm_align_rec = time_alignment(conf, rec, rec_tm_align, MAX_TIME_ALIGN);
        		rec = rec_tm_align;
        	}

        	// in case time alignment is set then it will loop over all
        	// the sub-samples, otherwise it will just loop once over the
        	// record that was read from file as is
        	for (i = 0; i < num_tm_align_rec; i++) {


        		if(!first_tm){
        			first_tm = 1;

        			if(!start_ts){
        				start_ts = rec[i].start_tm;
        			}
        			// in case duration is specified without start time, it is taken from the start of the file
        			end_ts = conf->dur? (start_ts + conf->dur) : (uint64_t)-1;
        		}

        		if(rec[i].start_tm < start_ts || rec[i].start_tm > end_ts){
        			continue;
        		}

        		if(rec[i].epoch_len){
        			v_ingress_kbps  = rec[i].v_ingress_bytes * 8 / rec[i].epoch_len;
        			a_ingress_kbps  = rec[i].a_ingress_bytes * 8 / rec[i].epoch_len;
        		}
        		else{
        			v_ingress_kbps = a_ingress_kbps = 0;
        		}

                switch(conf->format){
                case FORMAT_CSV:
            		printf("%s"
            				"%"PRIu64",%d,%"PRIu32",%"PRIu32",%"PRIu32",%d,%d,%d"
    						",%d,%d,%d,%d,%d,%d,%d"
    						",%d,%d,%d,%d,%d,%d,%d,%d"
    						",%d,%d,%d\n",
    						label_buf,
    						rec[i].start_tm,
    						rec[i].epoch_len, rec[i].v_latest_pts, rec[i].v_last_dts, rec[i].a_last_dts,
    						v_ingress_kbps, a_ingress_kbps, rec[i].dec_sleep_tm,
    						rec[i].a_num_disc_millisec, rec[i].a_num_inject_millisec, rec[i].a_num_rd_calls, rec[i].a_num_rd, rec[i].a_num_wr,
    						rec[i].a_num_err, rec[i].v_min_pending, rec[i].v_max_pending, rec[i].v_num_disc_frame, rec[i].v_num_dup_frame,
    						rec[i].v_num_rd_calls, rec[i].v_num_rd, rec[i].v_num_wr, rec[i].v_num_err, rec[i].v_num_reinit_ctx,
    						rec[i].v_width, rec[i].v_height, rec[i].acc_param);

            		break;
                case FORMAT_PROTOBUF:
                	enc_protobuf_write_rec_to_file(label_buf, label_len, &rec[i], fileno(stdout));
                	break;
                case FORMAT_MSGPACK:
                	enc_msgpack_write_rec_to_file(label_buf, label_len, &rec[i], fileno(stdout));
                    break;
                default:
                    fprintf(stderr, "unsupported format %d\n", conf->format);
                    break;
                }

        		if (alt_file_fd > 0) {
        			switch(conf->alt_file_format) {
        			case ALT_FILE_FORMAT_PROTOBUF:
        				enc_protobuf_write_rec_to_file(label_buf, label_len, &rec[i], alt_file_fd);
        				break;
        			case ALT_FILE_FORMAT_MSGPACK:
        				enc_msgpack_write_rec_to_file(label_buf, label_len, &rec[i], alt_file_fd);
        				break;
        			}
        		}
        	}
        }

        if(n % sizeof(xcode_dec_bin_log_record_t)){
            memmove(buf, buf + sizeof(xcode_dec_bin_log_record_t) * num_rec, n % sizeof(xcode_dec_bin_log_record_t));
            first = buf + (n % sizeof(xcode_dec_bin_log_record_t));
            len = sizeof(buf) - (n % sizeof(xcode_dec_bin_log_record_t));
        }
        else{
            first = buf;
            len = sizeof(buf);
        }
    }

    return 0;
}

#if defined(__APPLE__) || defined(__NetBSD__)
#define st_atim st_atimespec
#define st_ctim st_ctimespec
#define st_mtim st_mtimespec
#endif

static void
process_dirs(decgrep_conf_t *conf)
{
    int                       fd, i, j, n;
    int                       alt_file_fd;
    int                       rc;
    DIR                       *d;
    struct dirent             *dir;
    struct stat               sb;
    char                      files[MAX_FILES][sizeof(((struct dirent*)0)->d_name)];
    int                       files_dir[MAX_FILES];
    time_t                    files_mtime[MAX_FILES];
    char                      sep;
    char                      full_path[4096];

    memset(files, 0, sizeof(files));
    memset(files_dir, 0, sizeof(files_dir));
    memset(files_mtime, 0, sizeof(files_mtime));
    fd = alt_file_fd = 0;

    // iterate over all the configured directories to scan
    for (i = 0; i < conf->num_dir; i++) {

    	if (chdir(conf->directories[i]) < 0) {
    		fprintf(stderr, "failed to chdir dir=%s err=%s\n",
    				conf->directories[i], strerror(errno));
    		continue;
    	}

    	errno = 0;
        d = opendir(conf->directories[i]);
        if (d)
        {
        	// iterate over all the files in the directory
        	errno = 0;
            while ((dir = readdir(d)) != NULL)
            {
            	// match file name using wildcard expression
            	if (fnmatch(conf->filename, dir->d_name, FNM_FILE_NAME) == 0) {

            		// if it matches get the file type and modification time
            		memset(&sb, 0, sizeof(sb));
            		rc = stat(dir->d_name, &sb);

            		if (rc < 0) {
                    	fprintf(stderr, "failed to stat file dir=%s file=%s err=%s\n",
                    			conf->directories[i], dir->d_name, strerror(errno));
                    	continue;
            		}

                	// if it isn't a regular file skip it
                	if (!S_ISREG(sb.st_mode)) {
                		continue;
                	}

                	// add it to the list of files to scan (most recently updated at index zero)
            		for (j = 0; j < MAX_FILES; j++) {
                		if (sb.st_mtim.tv_sec > files_mtime[j]) {
                			if (j + 1 < MAX_FILES) {
                				memmove(&files_dir[j + 1], &files_dir[j], sizeof(files_dir[0]) * (MAX_FILES - j - 1));
                				memmove(&files_mtime[j + 1], &files_mtime[j], sizeof(files_mtime[0]) * (MAX_FILES - j - 1));
                				memmove(files[j + 1], files[j], sizeof(files[0]) * (MAX_FILES - j - 1));
                			}
                			files_dir[j]   = i;
                			files_mtime[j] = sb.st_mtim.tv_sec;
                			strcpy(files[j], dir->d_name);
                			break;
                		}
            		}
            	}
            }

            if (errno) {
            	fprintf(stderr, "failed to read dir %s. err=%s\n",
            			conf->directories[i], strerror(errno));
            }

            closedir(d);
        } else {
        	fprintf(stderr, "failed to open %s. err=%s\n",
        			conf->directories[i], strerror(errno));
        }
    }

#ifdef _WIN32
    sep = '\\';
#else
    sep = '/';
#endif

    if (conf->alt_file) {
        alt_file_fd = open(conf->alt_file, O_WRONLY|O_CREAT|O_TRUNC, 0666);
        if (alt_file_fd < 0) {
            fprintf(stderr, "failed to open %s. err=%s\n",
            		conf->alt_file, strerror(errno));
            exit(1);
        }
    }

    // go over the list of files from oldest to most recent
    // and produce the report based of the configuration
    for (i = MAX_FILES - 1; i >= 0; i--) {
    	// if not set then skip it
    	if (!files[i][0]) {
    		continue;
    	}

    	n = snprintf(full_path, sizeof(full_path),"%s%c%s",
    			conf->directories[files_dir[i]], sep, files[i]);

    	if ((size_t)n >= sizeof(full_path)) {
    		fprintf(stderr, "file path too long. path=%s", full_path);
    		continue;
    	}

        fd = open(full_path, O_RDONLY);

        if(fd < 0){
            fprintf(stderr, "failed to open %s. err=%s\n", full_path, strerror(errno));
            continue;
        }

        switch(conf->format){
        case FORMAT_TABLE:
            format_1(fd, alt_file_fd, conf);
            break;
        case FORMAT_CSV:
        case FORMAT_PROTOBUF:
        case FORMAT_MSGPACK:
            format_2(fd, alt_file_fd, conf);
            break;
        default:
            fprintf(stderr, "unsupported format %d\n", conf->format);
            break;
        }

        close(fd);

        conf->header  = 0; // header row should only be produced once
        conf->verbose = 0; // header description should only be produced once
    }


    if (alt_file_fd > 0) {
    	close(alt_file_fd);
    }
}

int main(int argc, char * const argv[])
{
	decgrep_conf_t            conf;
    int                       c;
    int                       fd;
    int                       alt_file_fd;
    int                       rc;

    static char* usage =
            "usage: %s [ -f <format> ] [ -v ] [ -H ] [ -l label ] [ -s <start timestamp> [-d <duration in ms>]] \n"
    		"          [ -t <interval ms> ] [ -p <protobuf file>] [ -a <directory> [ -a <directory> ] ...] [ -D ] log_name\n"
    		"-a <dir>           - one or more directories to look for files. If specified then log_name should be the wildcard pattern\n"
    		"                     to specify multiple directories use -a <dir1> -a <dir2>... \n"
    		"                   - the parser will parse all the files that match the pattern in the specified directories (sort by date and time)\n"
            "log_name           - full path to the log file to parse or wildcard pattern in case -a is used\n"
    		"-v                 - verbose i.e. add column description\n"
    		"-H                 - omit header line\n"
    		"-D                 - remove duplicate records\n"
    		"-l <label>         - add the specified label for every row as first field\n"
            "-f <format>        - format:\n"
            "                     1 = Table with headers and date conversion (default)\n"
            "                     2 = CSV with no headers and no date conversion\n"
            "                     3 = protobuf to stdout\n"
            "                     4 = msgpack to stdout\n"
            "-s <timestamp>     - start timestamp\n"
            "-d <duration>      - duration in milliseconds (if only dur is specified it is taken from the start of the stream)\n"
            "-p <protobuf file> - optional file path into which the processed records will be stored as protobuf records\n"
            "-m <msgpack file>  - optional file path into which the processed records will be stored as msgpack records\n"
    		"                     NOTE: either -p or -m can be specified but not both\n"
    		"-t <interval>      - optional align timestamps to fixed intervals in milliseconds\n"
            ;

    alt_file_fd = 0;
    memset(&conf, 0, sizeof(conf));
    conf.header = 1;
    conf.format = 1;

    while ((c = getopt (argc, argv, "vf:s:d:l:p:a:HDt:m:")) != -1)
    {
        switch(c){
        case 'v':
            conf.verbose = 1;
            break;
        case 'H':
        	conf.header = 0;
        	break;
        case 'D':
        	conf.dedup = 1;
        	break;
        case 'l':
        	conf.label = optarg;
            break;
        case 'f':
        	conf.format = atoi(optarg);
            break;
        case 's':
        	conf.start_ts = strtoull(optarg, NULL, 0);
            break;
        case 'd':
        	conf.dur = strtoull(optarg, NULL, 0);
            break;
        case 'p':
        	conf.alt_file = optarg;
        	conf.alt_file_format = ALT_FILE_FORMAT_PROTOBUF;
            break;
        case 'm':
        	conf.alt_file = optarg;
        	conf.alt_file_format = ALT_FILE_FORMAT_MSGPACK;
            break;
        case 'a':
        	if (conf.num_dir < MAX_DIR) {
        		conf.directories[conf.num_dir++] = optarg;
        	} else {
        		perror("too many directories\n");
        		exit(1);
        	}
        	break;
        case 't':
        	conf.time_align = atoi(optarg);
        	break;
        case '?':
            printf(usage, argv[0]);
            exit(0);
        }
    }

    if ((optind+1) > argc){
        fprintf(stderr, "please specify file name\n");
        fprintf(stderr, usage, argv[0]);
        exit(1);
    }

    conf.filename = argv[optind];

    if (conf.num_dir) {
    	process_dirs(&conf);
    	return 0;
    }

    fd = open(conf.filename, O_RDONLY);

    if(fd < 0){
        fprintf(stderr, "failed to open %s. err=%s\n", conf.filename, strerror(errno));
        exit(1);
    }

    if (conf.alt_file) {
        alt_file_fd = open(conf.alt_file, O_WRONLY|O_CREAT|O_TRUNC, 0666);
        if (alt_file_fd < 0) {
            fprintf(stderr, "failed to open %s. err=%s\n", conf.alt_file, strerror(errno));
            exit(1);
        }
    }

    switch(conf.format){
    case FORMAT_TABLE:
        rc = format_1(fd, alt_file_fd, &conf);
        break;
    case FORMAT_CSV:
    case FORMAT_PROTOBUF:
    case FORMAT_MSGPACK:
        rc = format_2(fd, alt_file_fd, &conf);
        break;
    default:
        fprintf(stderr, "unsupported format %d\n", conf.format);
        rc = 1;
        break;
    }

    close(fd);

    if (alt_file_fd > 0) {
        close(alt_file_fd);
    }
    return rc;
}

