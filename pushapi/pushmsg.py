#!/usr/bin/python2.6
import requests
import json
import sys

svr = sys.argv[1]
appid = sys.argv[2]
content = sys.argv[3]

d = {
	"token": "10294048a2KW5Hm1RDXlm1s0eZ1H99wbOwKfdNyn9AE9zafzle81wgFw0Of9L4BAn01m3m3aLM",
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

r = requests.post("%s/v1/push/message"%svr, data=json.dumps(d))
print r.status_code, r.text



