import json
import requests

def test():
    d = {
        "notices": [{
            "id": 1,
            "token": "t123",
            "appid": "appid_0a0e3404f5c648fc8c57dab52f871053",
            "msg_type": 1,
            "push_type": 3,
            "platform": "android",
            "tags": "1,2,3",
            "content": "xxxxxxxxxxxx",
            "ttl": 1800,    
        },
        {
            "id": 2,
            "token": "t123",
            "appid": "appid_0a0e3404f5c648fc8c57dab52f871053",
            "msg_type": 1,
            "push_type": 3,
            "platform": "android",
            "tags": "2,3,4",
            "content": "yyyyyyy",
        },
        ]        
    }
    
    r = requests.post("http://127.0.0.1:9999/api/v1/notify", data=json.dumps(d), headers={"content-type": "applcation/json"})
    print r.status_code, r.text


if __name__ == "__main__":
    test()
