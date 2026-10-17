# usage: python3 seed-sensitivity.py [seeds=8]   (server on :8091)
# For each challenge, runs the reference solution on N seeds and reports how stable pass/stars/p95/success are.
import json, sys, urllib.request, statistics as st
B = 'http://localhost:8091/api/'
N = int(sys.argv[1]) if len(sys.argv) > 1 else 8
get = lambda p: json.load(urllib.request.urlopen(B + p))
def run(d, sid, seed):
    r = urllib.request.Request(B + 'run', json.dumps({'diagram': d, 'scenarioId': sid, 'seed': seed}).encode(), {'Content-Type': 'application/json'})
    return json.load(urllib.request.urlopen(r))['score']
L = get('scenarios'); L = L if isinstance(L, list) else L['scenarios']
print(f"{'challenge':20} {'pass':>5} {'stars':>9} {'p95 mean':>9} {'p95 cv':>7} {'succ min':>9}")
for x in L:
    sid = x['id']; sol = get(f'scenarios/{sid}/solution'); sol = sol.get('diagram', sol)
    sc = [run(sol, sid, s) for s in range(1, N + 1)]
    p95 = [s['run']['p95'] for s in sc]; rate = [s['run']['ok'] / max(1, s['run']['resolved']) for s in sc]
    stars = ''.join(str(s['stars']) for s in sc)
    print(f"{sid:20} {sum(s['pass'] for s in sc):>2}/{N:<2} {stars:>9} {st.mean(p95):9.0f} {st.pstdev(p95)/max(1e-9, st.mean(p95)):7.2f} {min(rate):9.3f}")
