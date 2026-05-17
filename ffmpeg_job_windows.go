//go:build windows

package main

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	ffmpegJobMu sync.Mutex
	ffmpegJob   windows.Handle
)

// initFFmpegJob creates a job object that kills all assigned children when the
// job handle is closed (including when this process exits).
func initFFmpegJob() error {
	ffmpegJobMu.Lock()
	defer ffmpegJobMu.Unlock()

	if ffmpegJob != 0 {
		return nil
	}

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	_, err = windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		_ = windows.CloseHandle(job)
		return err
	}

	ffmpegJob = job
	return nil
}

// assignChildToFFmpegJob adds a child process to the FFmpeg job object.
func assignChildToFFmpegJob(pid int) error {
	ffmpegJobMu.Lock()
	job := ffmpegJob
	ffmpegJobMu.Unlock()

	if job == 0 || pid <= 0 {
		return nil
	}

	const access = windows.PROCESS_TERMINATE | windows.PROCESS_SET_QUOTA
	ph, err := windows.OpenProcess(access, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(ph)

	return windows.AssignProcessToJobObject(job, ph)
}

// closeFFmpegJob closes the job handle, terminating any remaining children.
func closeFFmpegJob() {
	ffmpegJobMu.Lock()
	defer ffmpegJobMu.Unlock()
	if ffmpegJob != 0 {
		_ = windows.CloseHandle(ffmpegJob)
		ffmpegJob = 0
	}
}
