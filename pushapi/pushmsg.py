#!/usr/bin/python2.6
#-*- encoding:utf8 -*-

import requests
import json
import sys

svr = sys.argv[1]
userid = sys.argv[2]
appid = sys.argv[3]
msg_type = int(sys.argv[4])
content = sys.argv[5]

d = {
	"token": "102304f687BrX9DhNzo2LnEm1qjEpRrhhIqm1DqGyWbXQaEPUNMInqXcO7s2bChpFIeYz1Xq",
	"userid" : userid,
	"appid": appid,
	"msg_type": msg_type,
	"push_type": 2,
	"push_params" : {
		"regid" : ["6523091263591782ce8305975344b4ef0377a281"]
	},
	"content": content,
	"platform": "all",
	"options": {"ttl": 864000},
}

r = requests.post("%s/api/v1/message"%svr, data=json.dumps(d))
print r.status_code, r.text

