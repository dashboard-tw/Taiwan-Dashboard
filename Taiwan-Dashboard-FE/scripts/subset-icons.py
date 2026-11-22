"""Material Icons Round 只保留網站用到的圖示。

新增圖示後在 frontend 資料夾執行：
    pip install fonttools brotli
    python scripts/subset-icons.py [https://dashboard.tw]

會掃描 src 裡所有可能是圖示名稱的字，加上 API 回傳的主題圖示，
輸出 src/assets/fonts/material-icons-round-subset.woff2。
"""

import glob
import json
import re
import sys
import urllib.request

from fontTools import subset
from fontTools.ttLib import TTFont

SRC = "node_modules/@fontsource/material-icons-round/files/material-icons-round-latin-400-normal.woff2"
OUT = "src/assets/fonts/material-icons-round-subset.woff2"
API = sys.argv[1] if len(sys.argv) > 1 else "https://dashboard.tw"


def ligatures(font):
    rev = {v: chr(k) for k, v in font.getBestCmap().items()}
    for lookup in font["GSUB"].table.LookupList.Lookup:
        for st in lookup.SubTable:
            table = st.ExtSubTable if lookup.LookupType == 7 else st
            if hasattr(table, "ligatures"):
                yield table, rev


def lig_name(rev, first, lig):
    if not all(c in rev for c in lig.Component):
        return None
    return rev[first] + "".join(rev[c] for c in lig.Component)


def wanted_names():
    words = set()
    for path in glob.glob("src/**/*.*", recursive=True):
        if path.endswith((".vue", ".ts")) and ".test." not in path:
            with open(path, encoding="utf-8") as f:
                words |= set(re.findall(r"[a-z][a-z0-9_]+", f.read()))
    req = urllib.request.Request(API + "/api/v1/dashboards", headers={"User-Agent": "Mozilla/5.0"})
    with urllib.request.urlopen(req) as r:
        for d in json.load(r)["data"]:
            if d.get("icon"):
                words.add(d["icon"])
    return words


def main():
    font = TTFont(SRC)
    wanted = wanted_names()
    keep = set()
    for table, rev in ligatures(font):
        for first in list(table.ligatures):
            ligs = [l for l in table.ligatures[first] if lig_name(rev, first, l) in wanted]
            if ligs:
                table.ligatures[first] = ligs
                keep.update(l.LigGlyph for l in ligs)
            else:
                del table.ligatures[first]

    opts = subset.Options()
    opts.layout_features = ["liga", "rlig", "calt", "ccmp"]
    opts.flavor = "woff2"
    opts.notdef_outline = True
    sub = subset.Subsetter(opts)
    sub.populate(unicodes=[ord(c) for c in "abcdefghijklmnopqrstuvwxyz0123456789_ "], glyphs=sorted(keep))
    sub.subset(font)
    font.flavor = "woff2"
    font.save(OUT)
    print(f"{len(keep)} 個圖示，輸出到 {OUT}")


if __name__ == "__main__":
    main()
