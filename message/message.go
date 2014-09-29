package message

type PushMessage struct {
	MsgId		int64	`json:"msgid"`
	AppId		string	`json:"appid"`
	Type		int	`json:"type"`  //1: notification  2:app message
	Content		string	`json:"content"`
}

type PushReplyMessage struct {
	MsgId	int64	`json:"msgid"`
	AppId	string	`json:"appid"`
	RegId	string	`json:"regid"`
}

type RawMessage struct {
	MsgId		int64		`json:"msgid"`
	AppId		string		`json:"appid"`
	CTime		int64		`json:"ctime"`
	Platform	string		`json:"platform"`
	PushType	int		`json:"push_type"`
	PushParams	interface{}	`json:"push_params"`
	Content		string		`json:"content"`
	Options		interface{}	`json:"options"`
}
