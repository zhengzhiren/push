#!/usr/bin/python2.6
import requests
import json
import sys

svr = sys.argv[1]
userid = sys.argv[2]
appid = sys.argv[3]
content = sys.argv[4]

d = {
	"token": "102304f687BrX9DhNzo2LnEm1qjEpRrhhIqm1DqGyWbXQaEPUNMInqXcO7s2bChpFIeYz1Xq",
	"userid" : userid,
	"appid": appid,
	"msg_type": 1,
	"push_type": 1,
	"push_params" : {
		"userid" : ["letv_56855159"]
	},
	"content": content,
	"platform": "all",
	"options": {"ttl": 864000},
}

r = requests.post("%s/v1/push/message"%svr, data=json.dumps(d))
print r.status_code, r.text



