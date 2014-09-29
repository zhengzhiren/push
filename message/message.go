package message

type PushMessage struct {
	MsgId		int64	`json:"msgid"`
	AppId		string	`json:"appid"`
	Type		int	`json:"type"`  //1: notification  2:app message
	Content		string	`json:"content"`
}

type RawMessage struct {
	MsgId		int64		`json:"msgid"`
	AppId		string		`json:"appid"`
	CTime		int64		`json:"ctime"`
	Platform	string		`json:"platform"`
	MsgType		int		`json:"msg_type"` //消息类型: 1通知 2消息
	PushType	int		`json:"push_type"`//发送类型: 1广播 2单播 3组播
	Content		string		`json:"content"`
	Options		interface{}	`json:"options"` //可选 title, ttl(消息生命周期), push_params(单播devid 组播tag list,alias list), timing(定时发送时间), custom_content等等
}
