"""网页版和桌面版共用的对话存档（blueprint/web/conversations.py）。"""

import json
import threading
import urllib.request
import zipfile
import io
from urllib.parse import quote

import pytest

from blueprint.handoff import build_handoff
from blueprint.web import conversations
from blueprint.web.server import App, create_server

IDEA = "家长拍一张玩具合影，AI 生成可以亲子共同演绎的剧情脚本。照片涉及隐私，默认不保存。"


def session(text="3 岁"):
    return {"messages": [
        {"id": "m1", "role": "user", "parts": [{"type": "text", "text": text}]},
        {"id": "m2", "role": "assistant", "parts": [{"type": "text", "text": "记下了"},
                                                    {"type": "data", "name": "play_feedback", "data": {}}]},
    ], "chips": ["跳过这题"]}


@pytest.fixture
def ws(tmp_path, monkeypatch):
    for name in ("ANTHROPIC_API_KEY", "OPENAI_API_KEY", "API_FORMAT"):
        monkeypatch.delenv(name, raising=False)
    app = App(tmp_path / "out", env_path=tmp_path / ".env", mock=True)
    res = app.create_product({"name": "玩具剧场", "idea": IDEA})
    job = app.jobs.get(res["job"]["id"])
    for _ in range(200):
        if job.status != "running":
            break
        threading.Event().wait(0.05)
    assert job.status == "done", job.error
    return app, app.out_root / res["slug"]


def test_saved_sessions_come_back(ws):
    _, path = ws
    for kind in conversations.KINDS:
        conversations.save(path, kind, {"session": session(kind)})
    conversations.save(path, "notes", {"notes": [{"id": "m2", "said": "你好", "reply": "嗨", "comment": "热情点"}]})
    got = conversations.load(path)
    for kind in conversations.KINDS:
        assert got[kind]["messages"][0]["parts"][0]["text"] == kind
    assert got["notes"][0]["comment"] == "热情点"


def test_stale_sessions_are_not_returned(ws):
    _, path = ws
    for kind in conversations.KINDS:
        conversations.save(path, kind, {"session": session()})
    # 需求文档升版：加新需求的对话作废
    prd = path / "PRD.md"
    prd.write_text(prd.read_text(encoding="utf-8").replace("| 当前版本 | V1 |", "| 当前版本 | V2 |"), encoding="utf-8")
    # 方案的提示词变了：试玩的对话作废
    plan = json.loads((path / "plan.json").read_text(encoding="utf-8"))
    plan.setdefault("design", {})["system_prompt"] = "换了一版提示词"
    (path / "plan.json").write_text(json.dumps(plan, ensure_ascii=False), encoding="utf-8")
    got = conversations.load(path)
    assert got["revise"] is None and got["play"] is None
    assert got["answers"] is not None          # 待确认问题没变，回答问题的对话还在


def test_answers_session_goes_stale_when_questions_change(ws, monkeypatch):
    _, path = ws
    conversations.save(path, "answers", {"session": session()})
    real = conversations.product_detail

    def changed(p):
        d = real(p)
        d["plan"] = {**d["plan"], "unknowns": ["全新的问题？"]}
        return d

    monkeypatch.setattr(conversations, "product_detail", changed)
    assert conversations.load(path)["answers"] is None


def test_null_session_deletes_and_empty_file_is_removed(ws):
    _, path = ws
    conversations.save(path, "answers", {"session": session()})
    assert (path / conversations.FILE).is_file()
    conversations.save(path, "answers", {"session": None})
    assert conversations.load(path)["answers"] is None
    assert not (path / conversations.FILE).exists()


def test_bad_input_is_rejected(ws):
    _, path = ws
    with pytest.raises(ValueError):
        conversations.save(path, "answers", {"session": {"messages": [{"role": "system", "parts": []}]}})
    with pytest.raises(ValueError):
        conversations.save(path, "notes", {"notes": "x"})
    with pytest.raises(LookupError):
        conversations.save(path, "secrets", {"session": session()})


def test_conversations_never_go_into_the_handoff(ws):
    _, path = ws
    conversations.save(path, "answers", {"session": session("不该出现在交接包里")})
    names = zipfile.ZipFile(io.BytesIO(build_handoff(path))).namelist()
    assert not any("conversations" in n for n in names)


def test_conversations_over_http(ws):
    app, path = ws
    srv = create_server(app, port=0)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    base = f"http://127.0.0.1:{srv.server_address[1]}/api/products/{quote(path.name)}/conversations"
    try:
        req = urllib.request.Request(base + "/answers", data=json.dumps({"session": session()}).encode(),
                                     headers={"Content-Type": "application/json"}, method="POST")
        with urllib.request.urlopen(req, timeout=5) as res:
            assert json.loads(res.read())["ok"] is True
        with urllib.request.urlopen(base, timeout=5) as res:
            got = json.loads(res.read())
        assert got["answers"]["chips"] == ["跳过这题"] and got["revise"] is None and got["notes"] == []
    finally:
        srv.shutdown()
        srv.server_close()
