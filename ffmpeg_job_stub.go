//go:build !windows

package main

func initFFmpegJob() error { return nil }

func assignChildToFFmpegJob(pid int) error { return nil }

func closeFFmpegJob() {}
