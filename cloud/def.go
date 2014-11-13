package cloud

const (
	ERR_NOERROR     = 10000
	ERR_CMD_TIMEOUT = 20000
)

type ApiStatus struct {
	ErrNo  int    `json:"errno"`
	ErrMsg string `json:"errmsg,omitempty"`
}

//type ApiResponse struct {
//	ApiStatus
//	Data interface{} `json:"data,omitempty"`
//}

type ApiResponse struct {
	ErrNo  int         `json:"errno"`
	ErrMsg string      `json:"errmsg,omitempty"`
	Data   interface{} `json:"data,omitempty"`
}
