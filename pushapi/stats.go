package main

import (
	"sync"
	"sync/atomic"
	"time"
)

type CtrlStats struct {
	Cmd               uint32
	CmdSuccess        uint32
	CmdTimeout        uint32
	CmdOffline        uint32
	CmdInvalidService uint32
	CmdOtherError     uint32
}

type Statistics struct {
	lock      *sync.RWMutex
	StartTime time.Time

	PushMsg uint32

	CtrlStatistics     map[string]*CtrlStats
	QueryOnlineDevices uint32
	QueryDeviceInfo    uint32
}

var (
	Stats *Statistics = nil
)

func newStats() {
	Stats = &Statistics{
		lock:           new(sync.RWMutex),
		CtrlStatistics: make(map[string]*CtrlStats),
		StartTime:      time.Now(),
	}
}

func (this *Statistics) pushMsg() {
	atomic.AddUint32(&this.PushMsg, 1)
}

func (this *Statistics) cmd(service string) {
	this.lock.Lock()
	stats, ok := this.CtrlStatistics[service]
	if !ok {
		stats = &CtrlStats{}
		this.CtrlStatistics[service] = stats
	}
	stats.Cmd++
	this.lock.Unlock()
}

func (this *Statistics) cmdSuccess(service string) {
	this.lock.Lock()
	stats, ok := this.CtrlStatistics[service]
	if !ok {
		stats = &CtrlStats{}
		this.CtrlStatistics[service] = stats
	}
	stats.CmdSuccess++
	this.lock.Unlock()
}

func (this *Statistics) cmdTimeout(service string) {
	this.lock.Lock()
	stats, ok := this.CtrlStatistics[service]
	if !ok {
		stats = &CtrlStats{}
		this.CtrlStatistics[service] = stats
	}
	stats.CmdTimeout++
	this.lock.Unlock()
}

func (this *Statistics) cmdOffline(service string) {
	this.lock.Lock()
	stats, ok := this.CtrlStatistics[service]
	if !ok {
		stats = &CtrlStats{}
		this.CtrlStatistics[service] = stats
	}
	stats.CmdOffline++
	this.lock.Unlock()
}

func (this *Statistics) cmdInvalidService(service string) {
	this.lock.Lock()
	stats, ok := this.CtrlStatistics[service]
	if !ok {
		stats = &CtrlStats{}
		this.CtrlStatistics[service] = stats
	}
	stats.CmdInvalidService++
	this.lock.Unlock()
}

func (this *Statistics) cmdOtherError(service string) {
	this.lock.Lock()
	stats, ok := this.CtrlStatistics[service]
	if !ok {
		stats = &CtrlStats{}
		this.CtrlStatistics[service] = stats
	}
	stats.CmdOtherError++
	this.lock.Unlock()
}

func (this *Statistics) queryOnlineDevices() {
	atomic.AddUint32(&this.QueryOnlineDevices, 1)
}

func (this *Statistics) queryDeviceInfo() {
	atomic.AddUint32(&this.QueryDeviceInfo, 1)
}
