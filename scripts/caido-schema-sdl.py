"""Dump a live Caido server's GraphQL schema as SDL via introspection.

The repo's test schema (internal/testutil/caido-schema-v*.graphql) comes
from this script, not from caido/schemas: the published 0.59.0 file uses
different names from the released 0.59.0 server (HttpOnePipeline vs
HttpPipeline, allowlist/denylist vs captureList/passthroughList).

usage: CAIDO_URL=http://127.0.0.1:8080 CAIDO_TOKEN=<access token> \
         python3 -I scripts/caido-schema-sdl.py [published.graphql] > schema.graphql

published.graphql is only consulted for @oneOf markers when the server
does not expose __Type.isOneOf (matched by input type name).
"""
import json, os, re, sys, urllib.request
URL = os.environ.get("CAIDO_URL", "http://127.0.0.1:8080").rstrip("/") + "/graphql"
tok = os.environ["CAIDO_TOKEN"]
def post(q):
    req = urllib.request.Request(URL, data=json.dumps({"query": q}).encode(),
        headers={"Content-Type": "application/json", "Authorization": "Bearer " + tok})
    return json.loads(urllib.request.urlopen(req, timeout=60).read())
REF = "kind name ofType { kind name ofType { kind name ofType { kind name ofType { kind name } } } }"
def query(oneof):
    return """{ __schema { queryType{name} mutationType{name} subscriptionType{name}
      types { kind name %s
        fields(includeDeprecated:true){ name isDeprecated deprecationReason args{ name defaultValue type{%s} } type{%s} }
        inputFields{ name defaultValue type{%s} }
        interfaces{ name } possibleTypes{ name } enumValues(includeDeprecated:true){ name } } } }""" % (
        "isOneOf" if oneof else "", REF, REF, REF)
r = post(query(True)); has_oneof = "errors" not in r
if not has_oneof:
    r = post(query(False))
s = r["data"]["__schema"]
published_oneof = set()
if not has_oneof and len(sys.argv) > 1:
    published_oneof = set(re.findall(r"^input (\w+) @oneOf", open(sys.argv[1]).read(), re.M))
def ref(t):
    if t["kind"] == "NON_NULL": return ref(t["ofType"]) + "!"
    if t["kind"] == "LIST": return "[" + ref(t["ofType"]) + "]"
    return t["name"]
def args(a):
    if not a: return ""
    return "(" + ", ".join(x["name"] + ": " + ref(x["type"]) + (" = " + x["defaultValue"] if x["defaultValue"] is not None else "") for x in a) + ")"
BUILTIN = {"String", "Int", "Float", "Boolean", "ID"}
version = post("{ runtime { version } }")["data"]["runtime"]["version"]
out = [f"# Introspected from a live Caido {version} server by scripts/caido-schema-sdl.py.", ""]
out.append("directive @oneOf on INPUT_OBJECT\n")
roots = {s["queryType"]["name"]: "query", (s["mutationType"] or {}).get("name"): "mutation", (s["subscriptionType"] or {}).get("name"): "subscription"}
out.append("schema {\n" + "".join(f"  {v}: {k}\n" for k, v in roots.items() if k) + "}\n")
for t in sorted(s["types"], key=lambda t: t["name"]):
    n, k = t["name"], t["kind"]
    if n.startswith("__") or n in BUILTIN: continue
    if k == "SCALAR": out.append(f"scalar {n}\n"); continue
    if k == "ENUM": out.append(f"enum {n} {{\n" + "".join(f"  {e['name']}\n" for e in t["enumValues"]) + "}\n"); continue
    if k == "UNION": out.append(f"union {n} = " + " | ".join(p["name"] for p in t["possibleTypes"]) + "\n"); continue
    if k == "INPUT_OBJECT":
        oneof = t.get("isOneOf") if has_oneof else n in published_oneof
        out.append(f"input {n}{' @oneOf' if oneof else ''} {{\n" + "".join(
            f"  {f['name']}: {ref(f['type'])}" + (f" = {f['defaultValue']}" if f["defaultValue"] is not None else "") + "\n"
            for f in t["inputFields"]) + "}\n"); continue
    kw = "interface" if k == "INTERFACE" else "type"
    impl = (" implements " + " & ".join(i["name"] for i in t["interfaces"])) if t.get("interfaces") else ""
    body = ""
    for f in t["fields"]:
        dep = ""
        if f["isDeprecated"]:
            dep = " @deprecated" + (f"(reason: {json.dumps(f['deprecationReason'])})" if f["deprecationReason"] else "")
        body += f"  {f['name']}{args(f['args'])}: {ref(f['type'])}{dep}\n"
    out.append(f"{kw} {n}{impl} {{\n{body}}}\n")
print("\n".join(out))
print(f"# oneOf source: {'introspection' if has_oneof else 'published'}", file=sys.stderr)
