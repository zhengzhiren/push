import json
import requests

def test():
    d = {
        "notices": [{
            "id": 1,
            "appsec": "sk_ffLOQXYkeHpQlilnurdT",
            "appid": "id_8ac4be471b414877b362c5d63e59a212",
            "tags": "1,2,3",
            "push_msg" : {
                "msg_type": 2,
                "platform": "android",
                "content": "xxxxxxxxxxxx",
            },
        },
        {
            "id": 2,
            "appsec": "sk_ffLOQXYkeHpQlilnurdT",
            "appid": "",
            "push_msg" : {
                "msg_type": 2,
                "tags": "2,3,4",
                "content": "yyyyyyy",
            }
        },
        {
            "appsec": "appsec_trRhtZPGjfdMatwoFKQz",
            "appid": "appid_0a0e3404f5c648fc8c57dab52f871053",
            "msg_type": 2,
            "platform": "android",
            "tags": "3,4,5",
            "content": "zzzz",
        },
        {
            "id": 4,
            "appsec": "appsec_trRhtZPGjfdMatwoFKQz",
            "appid": "appid_0a0e3404f5c648fc8c57dab52f871053",
            "tags": "3,4,5",
            "push_msg" :{
                "msg_type": 2,
                "platform": "android",
                "content": "wwww",
                "options": {
                    "ttl": 1800,
                    "tts": 1800,
                }
            }
        },
        {
            "id": 5,
            "appsec": "sk_ffLOQXYkeHpQlilnurdT",
            "appid": "id_8ac4be471b414877b362c5d63e59a212",
            "tags": "3,4,5",
        },
        ]        
    }
    
    r = requests.post("http://127.0.0.1:50000/api/v1/notify", data=json.dumps(d), headers={"content-type": "applcation/json"})
    print r.status_code, r.text


if __name__ == "__main__":
    test()
