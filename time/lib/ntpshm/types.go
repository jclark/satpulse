//go:build ignore
// +build ignore

package ntpshm

/*
#include <sys/time.h>

struct shmTime {
	int      mode;
	int      count;
	time_t   clockTimeStampSec;
	int      clockTimeStampUSec;
	time_t   receiveTimeStampSec;
	int      receiveTimeStampUSec;
	int      leap;
	int      precision;
	int      nsamples;
	int      valid;
	int      clockTimeStampNSec;
	int      receiveTimeStampNSec;
	int      dummy[8];
};
*/
import "C"

type shmTime C.struct_shmTime

const expectedSize = C.sizeof_struct_shmTime
