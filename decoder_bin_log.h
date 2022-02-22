/*
 * decoder_bin_log.h
 *
 *  Created on: May 21, 2019
 *      Author: Amir Pauker
 *
 * definitions for decoder binary log structs.
 * the decoder frequently logs stats in binary format in order to save space and time
 */

#include <stdint.h>

#ifndef SRC_DECODER_BIN_LOG_H_
#define SRC_DECODER_BIN_LOG_H_

// NOTE: the order and type of the fields were carefully set to ensure compact struct
// we calculate the total required disk size is
// struct size x samples per hour x 24 hours x 30 concurrent streams x 4 (1 dec, 3 enc) x 30 days x compression rate (~0.5)
typedef struct {
    uint64_t                     start_tm;                     // the record start timestamp in milliseconds
    uint32_t                     v_latest_pts;                 // the most recent video PTS in the source buffer
    uint32_t                     v_last_dts;                   // the last video DTS read from the source buffer
    uint32_t                     a_last_dts;                   // the last audio DTS read from the source buffer
    uint32_t                     v_ingress_bytes;              // total video bytes received in epoch
    uint16_t                     a_ingress_bytes;              // total audio bytes received in epoch

    uint16_t                     epoch_len;                    // epoch duration in milliseconds
    uint16_t                     dec_sleep_tm;                 // total video decoding sleep time in milliseconds in epoch

    uint16_t                     a_num_disc_millisec;          // number of discard audio milliseconds in epoch
    uint16_t                     a_num_inject_millisec;        // number of injected audio silence milliseconds in epoch
    uint8_t                      a_num_rd_calls;               // number of calls to read audio
    uint8_t                      a_num_rd;                     // number of successful audio read encoded content calls in epoch
    uint8_t                      a_num_wr;                     // number of successful audio write decoded content calls in epoch
    uint8_t                      a_num_err;                    // number of errors decoding audio in epoch

    uint8_t                      v_max_pending;                // the max number of pending pictures in the source buffer in epoch
    uint8_t                      v_min_pending;                // the min number of pending pictures in the source buffer in epoch
    uint16_t                     v_num_wr;                     // number of successful video write decoded content calls in epoch
    uint8_t                      v_num_rd_calls;               // number of calls to read video
    uint8_t                      v_num_rd;                     // number of successful video read encoded content calls in epoch
    uint8_t                      v_num_disc_frame;             // number of discard video frames in epoch
    uint8_t                      v_num_dup_frame;              // number of duplicated video frames in epoch
    uint8_t                      v_num_err;                    // number of errors decoding video in epoch
    uint8_t                      v_num_reinit_ctx;             // number of video decode re-init context in epoch
    uint16_t                     v_width;                      // width of source video frame in pixels
    uint16_t                     v_height;                     // height of source video frame in pixels
    uint8_t                      acc_param;                    // an opaque value which determines the state of the room i.e. paid / public
} xcode_dec_bin_log_record_t;

typedef struct {
    char                         log_file_path[256];  // stores the name of the log file in case we need to rotate
    char                         done_file_dir[256];  // an optional directory into which closed logs should be moved
    uint64_t                     start_tm;            // start timestamp in milliseconds of the currently open file
    int                          fd;                  // the file into which samples are written once the record is full
    int                          min_dur_ms;          // files shorter than this number of milliseconds will not be moved to done directory
    int                          max_dur_ms;          // files longer than this number of milliseconds will be rotated and moved to done directory
    xcode_dec_bin_log_record_t   record;              // an instance of the binary log record
} xcode_dec_bin_log_ctx_t;

// the decoder collects stats in binary format and periodically dumps it to
// a file. This macro defines how often stats should be dump to the file
// NOTE: changing the interval affect the size of the fields of xcode_dec_bin_log_record_t
#define XCODE_DEC_BINARY_LOG_REC_INTERVALS 2000

#endif /* SRC_DECODER_BIN_LOG_H_ */