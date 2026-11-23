"""网页版和桌面版共用的对话存档：回答问题、加新需求、试玩三段对话，和试玩里记下的「这里不对」。

存在产品文件夹的 `.conversations.json`（交接包按清单打包，不会带走它）。哪段对话还能接着用由这里判断，
两个界面不用各算一遍：

- 回答问题：方案里的待确认问题变了（重新推导过），旧对话作废
- 加新需求：需求文档升了版，旧对话作废
- 试玩：方案里决定 Agent 行为的部分（提示词、工具、安全规则）变了，旧对话作废
- 「这里不对」的记录不随方案作废，带去加新需求或删掉才没

一段对话（session）的格式，两个界面都按它读写：

    {
      "messages": [
        {"id": "m1", "role": "user", "parts": [{"type": "text", "text": "……"}]},
        {"id": "m2", "role": "assistant", "parts": [
            {"type": "text", "text": "……"},
            {"type": "data", "name": "play_step", "data": {...}},      # 试玩里模拟的一步
            {"type": "data", "name": "play_feedback", "data": {}},     # 这条回复下面可以点「这里不对」
            {"type": "tool", "name": "confirm_answers", "args": {...}}  # 待确认的卡片
        ]}
      ],
      "chips": ["……"],            # 最近一次回复带的快捷回复
      "card": {...}               # 可选：卡片上用户改过、还没确认的内容
    }

「这里不对」：[{"id": 那条回复的 id, "said": 我说的, "reply": 它回的, "comment": 我希望}]
"""

from __future__ import annotations

import hashlib
import json
import threading
from pathlib import Path

from blueprint import playground
from blueprint.web.views import product_detail
from blueprint.workspace import current_version, read_prd

FILE = ".conversations.json"
KINDS = ("answers", "revise", "play")
MAX_MESSAGES = 400
_lock = threading.Lock()


def _stamps(ws: Path) -> dict[str, str]:
    """每种对话现在对应的「版本」：存下时记一份，读的时候对不上就说明旧了。"""
    detail = product_detail(ws)
    plan_view = detail.get("plan") or {}
    unknowns = plan_view.get("unknowns") or []
    answers = hashlib.sha1("\u0000".join(unknowns).encode("utf-8")).hexdigest()[:12] if unknowns else ""
    try:
        plan = json.loads((ws / "plan.json").read_text(encoding="utf-8"))
        play = playground.fingerprint(plan) if (plan.get("design") or {}).get("system_prompt") else ""
    except (OSError, ValueError):
        play = ""
    return {"answers": answers, "revise": f"v{current_version(read_prd(ws))}", "play": play}


def _read(ws: Path) -> dict:
    try:
        data = json.loads((ws / FILE).read_text(encoding="utf-8"))
        return data if isinstance(data, dict) else {}
    except (OSError, ValueError):
        return {}


def _write(ws: Path, data: dict) -> None:
    path = ws / FILE
    if not any(data.get(k) for k in (*KINDS, "notes")):
        path.unlink(missing_ok=True)
        return
    tmp = path.with_name(path.name + ".tmp")
    tmp.write_text(json.dumps(data, ensure_ascii=False), encoding="utf-8")
    tmp.replace(path)


def load(ws: Path) -> dict:
    """还能接着用的对话；旧了的不返回。"""
    with _lock:
        data = _read(ws)
    stamps = _stamps(ws)
    out: dict = {"notes": data.get("notes") if isinstance(data.get("notes"), list) else []}
    for kind in KINDS:
        entry = data.get(kind)
        ok = isinstance(entry, dict) and stamps[kind] and entry.get("stamp") == stamps[kind]
        out[kind] = entry.get("session") if ok else None
    return out


def _check_session(session) -> dict:
    if not isinstance(session, dict) or not isinstance(session.get("messages"), list):
        raise ValueError("对话格式不对")
    if len(session["messages"]) > MAX_MESSAGES:
        raise ValueError("对话太长了，存不下")
    for m in session["messages"]:
        if not isinstance(m, dict) or m.get("role") not in ("user", "assistant") or not isinstance(m.get("parts"), list):
            raise ValueError("对话格式不对")
    return session


def save(ws: Path, kind: str, body: dict) -> dict:
    """存一段对话（body.session 为 null 时删掉），或者存「这里不对」（kind=notes，body.notes）。"""
    if kind == "notes":
        notes = body.get("notes")
        if not isinstance(notes, list) or not all(isinstance(n, dict) for n in notes):
            raise ValueError("「这里不对」的记录格式不对")
        with _lock:
            data = _read(ws)
            data["notes"] = notes
            _write(ws, data)
        return {"ok": True}
    if kind not in KINDS:
        raise LookupError("没有这种对话")
    session = body.get("session")
    stamp = _stamps(ws)[kind]
    with _lock:
        data = _read(ws)
        if session is None:
            data.pop(kind, None)
        else:
            data[kind] = {"stamp": stamp, "session": _check_session(session)}
        _write(ws, data)
    return {"ok": True}
