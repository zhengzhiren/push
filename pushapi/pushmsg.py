#!/usr/bin/python2.6
import requests
import json
import sys

appid = sys.argv[1]
content = sys.argv[2]

d = {
	"token": "1023ac8290wpdgtEm30m29u1oBFuWkm12N6NdMnINCcBxbMqO0iIdsn7heM4lCm1selm1RFq4i0",
	"appid": appid,
	"msg_type": 1,
	"push_type": 3,
	"push_params" : {
		"userid" : ["letv_56855159"]
	},
	"content": content,
	"platform": "all",
	"options": {"ttl": 864000},
}

r = requests.post("http://127.0.0.1:8080/v1/push/message", data=json.dumps(d))
print r.status_code, r.text



