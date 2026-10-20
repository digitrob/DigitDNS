//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package service

import "syscall"

// processCPUSeconds returns total user+system CPU seconds consumed by the
// current process (BSD/Darwin implementation via getrusage).
//
// The Linux path reads /proc/self/stat, which does not exist here, so this
// build would otherwise fail its CPU sampling outright — the resolver would
// report no CPU figure at all on a macOS or *BSD host. getrusage is the
// portable BSD syscall that reports the same cumulative figures, in whole
// seconds plus microseconds, which Timeval.Seconds already sums.
func processCPUSeconds() (float64, error) {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0, err
	}
	return float64(ru.Utime.Sec+ru.Stime.Sec) + float64(ru.Utime.Usec+ru.Stime.Usec)/1e6, nil
}
