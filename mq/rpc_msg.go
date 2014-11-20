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
