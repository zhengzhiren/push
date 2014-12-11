package mq

type MQ_Msg_Crtl struct {
	DeviceId string `json:"dev_id"`
	Service  string `json:"svc"`
	Cmd      string `json:"cmd"`
}

type MQ_Msg_CtrlReply struct {
	Status int    `json:"status"`
	Result string `json:"result,omitempty"`
}

const (
	STATUS_SUCCESS = 0

	STATUS_INVALID_SERVICE = 1
	STATUS_EXCEPTION       = 2

	STATUS_NO_DEVICE    = -1
	STATUS_SEND_FAILED  = -2
	STATUS_SEND_TIMEOUT = -3
)
