from flask import Flask, request
import json

app = Flask(__name__)

@app.route("/api/v1/follow/users", methods=["GET"])
def test():
    tids = request.args.get("tagid")
    return json.dumps({"errno": 10000, "errmsg": "", "data": {1: {"uids":["letv_23212413"], "count":1, "app":""}}})

app.run(host="0.0.0.0", port=5000)
