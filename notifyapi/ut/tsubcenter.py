from flask import Flask, request
import json

app = Flask(__name__)

@app.route("/v1/subcenter", methods=["GET"])
def test():
    tids = request.args.get("tids")
    return json.dumps({"errno": 10000, "errmsg": "", "data": {"uids": ["letv_23212413"]}})

app.run(host="0.0.0.0", port=5000)
