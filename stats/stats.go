package stats

import (
	"strings"

	"github.com/chenyf/push/storage"
)

type CtrlStats struct {
	Cmd               int
	CmdSuccess        int
	CmdTimeout        int
	CmdOffline        int
	CmdInvalidService int
	CmdOtherError     int
}

type Stats struct {
	CtrlStatistics     map[string]*CtrlStats
	ReplyTooLate       int
	QueryOnlineDevices int
	QueryDeviceInfo    int
	Devices            map[string]int
	Comets             map[string]int
}

func GetStats() (*Stats, error) {
	stats := Stats{
		CtrlStatistics: make(map[string]*CtrlStats),
		Devices:        make(map[string]int),
		Comets:         make(map[string]int),
	}
	services, err := storage.Instance.GetStatsServices()
	if err != nil {
		return nil, err
	}
	for _, svc := range services {
		ctrlStats := CtrlStats{}

		ctrlStats.Cmd, _ = GetStatsCmd(svc)
		ctrlStats.CmdSuccess, _ = GetStatsCmdSuccess(svc)
		ctrlStats.CmdTimeout, _ = GetStatsCmdTimeout(svc)
		ctrlStats.CmdOffline, _ = GetStatsCmdOffline(svc)
		ctrlStats.CmdInvalidService, _ = GetStatsCmdInvalidService(svc)
		ctrlStats.CmdOtherError, _ = GetStatsCmdOtherError(svc)
		stats.CtrlStatistics[svc] = &ctrlStats
	}
	stats.QueryOnlineDevices, _ = GetStatsQueryOnlineDevices()
	stats.QueryDeviceInfo, _ = GetStatsQueryDeviceInfo()
	stats.ReplyTooLate, _ = GetStatsReplyTooLate()

	servers, _ := storage.Instance.GetServerNames()
	for _, server := range servers {
		ids, _ := storage.Instance.GetDeviceIds(server)
		stats.Comets[server] = len(ids)
		for _, id := range ids {
			devType := strings.Split(id, "-")[0]
			stats.Devices[devType]++
		}
	}

	return &stats, nil
}

func ClearStats() error {
	return storage.Instance.ClearStats()
}

func GetStatsServices() ([]string, error) {
	return storage.Instance.GetStatsServices()
}

func Cmd(service string) {
	storage.Instance.IncCmd(service)
}

func GetStatsCmd(service string) (int, error) {
	return storage.Instance.GetStatsCmd(service)
}

func CmdSuccess(service string) {
	storage.Instance.IncCmdSuccess(service)
}

func GetStatsCmdSuccess(service string) (int, error) {
	return storage.Instance.GetStatsCmdSuccess(service)
}

func CmdTimeout(service string) {
	storage.Instance.IncCmdTimeout(service)
}

func GetStatsCmdTimeout(service string) (int, error) {
	return storage.Instance.GetStatsCmdTimeout(service)
}

func CmdOffline(service string) {
	storage.Instance.IncCmdOffline(service)
}

func GetStatsCmdOffline(service string) (int, error) {
	return storage.Instance.GetStatsCmdOffline(service)
}

func CmdInvalidService(service string) {
	storage.Instance.IncCmdInvalidService(service)
}

func GetStatsCmdInvalidService(service string) (int, error) {
	return storage.Instance.GetStatsCmdInvalidService(service)
}

func CmdOtherError(service string) {
	storage.Instance.IncCmdOtherError(service)
}

func GetStatsCmdOtherError(service string) (int, error) {
	return storage.Instance.GetStatsCmdOtherError(service)
}

func QueryOnlineDevices() {
	storage.Instance.IncQueryOnlineDevices()
}

func GetStatsQueryOnlineDevices() (int, error) {
	return storage.Instance.GetStatsQueryOnlineDevices()
}

func QueryDeviceInfo() {
	storage.Instance.IncQueryDeviceInfo()
}

func GetStatsQueryDeviceInfo() (int, error) {
	return storage.Instance.GetStatsQueryDeviceInfo()
}

func ReplyTooLate() {
	storage.Instance.IncReplyTooLate()
}

func GetStatsReplyTooLate() (int, error) {
	return storage.Instance.GetStatsReplyTooLate()
}
