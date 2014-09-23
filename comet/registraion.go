package comet

import (
	"fmt"
	"github.com/chenyf/push/utils/safemap"
)

type App struct {
	AppKey	string
	DevId	string
	Tags	[]string
	Alias	*string
}

var (
	RegistrationsMap *safemap.SafeMap = safemap.NewSafeMap()
)

func RegId(devid string, appKey string) string {
	return fmt.Sprintf("%s_%s", devid, appKey)
}

