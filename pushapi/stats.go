package main

import (
	"sync/atomic"
)

type Statistics struct {
	PushMsg uint32

	Cmd                uint32
	CmdSuccess         uint32
	CmdTimeout         uint32
	CmdOffline         uint32
	CmdInvalidService  uint32
	CmdOtherError      uint32
	QueryOnlineDevices uint32
	QueryDeviceInfo    uint32
}

var (
	Stats Statistics
)

func newStats() {
	Stats = Statistics{}
}

func (this *Statistics) pushMsg() {
	atomic.AddUint32(&this.PushMsg, 1)
}

func (this *Statistics) cmd() {
	atomic.AddUint32(&this.Cmd, 1)
}

func (this *Statistics) cmdSuccess() {
	atomic.AddUint32(&this.CmdSuccess, 1)
}

func (this *Statistics) cmdTimeout() {
	atomic.AddUint32(&this.CmdTimeout, 1)
}

func (this *Statistics) cmdOffline() {
	atomic.AddUint32(&this.CmdOffline, 1)
}

func (this *Statistics) cmdInvalidService() {
	atomic.AddUint32(&this.CmdInvalidService, 1)
}

func (this *Statistics) cmdOtherError() {
	atomic.AddUint32(&this.CmdOtherError, 1)
}

func (this *Statistics) queryOnlineDevices() {
	atomic.AddUint32(&this.QueryOnlineDevices, 1)
}

func (this *Statistics) queryDeviceInfo() {
	atomic.AddUint32(&this.QueryDeviceInfo, 1)
}
