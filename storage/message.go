package storage

type RawMessage struct {
	MsgId		int64		`json:"msgid"`
	CTime		time.Time	`json:"ctime"`
	Platform	string		`json:"platform"`
	PushType	int			`json:"push_type"`
	PushParams	interface{}	`json:"push_params"`
	Content		string		`json:"content"`
	Options		interface{}	`json:"options"`
}

