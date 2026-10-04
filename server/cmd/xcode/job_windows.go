package main

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// killChildrenOnExit 把自己放進一個 kill-on-close 的 Job Object。之後啟動的 ffmpeg 會繼承這個 job，
// xcode.exe 不論怎麼結束（包括 taskkill /f），handle 一關，ffmpeg 就跟著被結束，不會在背景繼續轉碼。
func killChildrenOnExit() error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("job object: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return fmt.Errorf("job object: %w", err)
	}
	if err := windows.AssignProcessToJobObject(job, windows.CurrentProcess()); err != nil {
		windows.CloseHandle(job)
		return fmt.Errorf("job object: %w", err)
	}
	// 刻意不關 handle：它要活到行程結束
	return nil
}
