package cloud

const (
	ERR_NOERROR = 10000

	// remote control errors
	ERR_CMD_TIMEOUT         = 20000
	ERR_CMD_INVALID_SERVICE = 20001
	ERR_CMD_SDK_ERROR       = 20002
	ERR_CMD_OTHER           = 20040
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
