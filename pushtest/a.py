import hmac
import hashlib
import time
import random
import itertools
import pprint

permutations = {}  # cache all permutations

for k in itertools.permutations(range(7)):
	permutations[len(permutations)] = k

pprint.pprint(permutations)

def sign(path, query):
	uid = query.get('uid', 'uid')
	rid = query.get('rid', 'rid')
	tid = query.get('tid', 'tid')
	src = query.get('src', 'src')
	tm = query.get('tm', int(time.time()))
	pmtt = query.get('pmtt', random.randint(0, 5039))
	raw = [path, uid, rid, str(tid), src, str(tm), str(pmtt)]
	args = []
	for i in permutations[pmtt]:
		args.append(raw[i])

	key = 'xnRzFxoCDRVRU2mNQ7AoZ5MCxpAR7ntnmlgRGYav'
	return hmac.new(key, ''.join(args), hashlib.sha1).hexdigest()


s = time.time()
r = sign('/router/download/add', {
	'uid': 'u123456',
	'rid': 'r832453',
	'tid': 3,
	'src': 'letv',
	'tm': 1404216009,
	'pmtt': 5039,
	})

print('elapsed: %d(microseconds)' % ((time.time() - s) * 1000000))
assert r == 'b4f2a83a928cd5d1a3c0e782da63729cbd4dff22'

