"""Claude Code 插件（plugins/agent-workshop）：清单能被正确读取，技能自带的选型脚本和知识库与引擎保持一致。

插件装到用户机器上时只有 plugins/agent-workshop/ 这个目录，引擎代码不在。所以选型规则和组件知识
被复制进了插件；这里守住「复制品和引擎一字不差」，改了引擎忘了同步就会挂。
"""
import json
import random
import re
import subprocess
import sys
from pathlib import Path

import pytest

from blueprint.knowledge import ACCEPTANCE, COMPONENTS, FACT_KEYS, FEATURES, FIVE_ELEMENTS, LAYERS
from blueprint.rules import select_components

ROOT = Path(__file__).resolve().parents[1]
PLUGIN = ROOT / "plugins" / "agent-workshop"
SKILLS = PLUGIN / "skills"
SCRIPT = SKILLS / "agent-prd" / "scripts" / "select_components.py"


def _load_script():
    import importlib.util
    spec = importlib.util.spec_from_file_location("plugin_select_components", SCRIPT)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def _frontmatter(path: Path) -> dict[str, str]:
    text = path.read_text(encoding="utf-8")
    m = re.match(r"^---\n(.*?)\n---\n", text, re.S)
    assert m, f"{path} 缺少 YAML frontmatter"
    out = {}
    for line in m.group(1).splitlines():
        key, _, value = line.partition(":")
        out[key.strip()] = value.strip()
    return out


def test_marketplace_and_plugin_manifests_agree():
    market = json.loads((ROOT / ".claude-plugin" / "marketplace.json").read_text(encoding="utf-8"))
    plugin = json.loads((PLUGIN / ".claude-plugin" / "plugin.json").read_text(encoding="utf-8"))
    assert market["name"] and market["owner"]["name"]
    [entry] = market["plugins"]
    assert entry["name"] == plugin["name"] == "agent-workshop"     # 不一致会报 Plugin not found
    assert entry["source"].startswith("./") and ".." not in entry["source"]
    assert (ROOT / entry["source"]).resolve() == PLUGIN.resolve()
    assert re.fullmatch(r"\d+\.\d+\.\d+", plugin["version"])


@pytest.mark.parametrize("skill", ["agent-prd", "agent-builder"])
def test_skill_frontmatter_and_links(skill):
    path = SKILLS / skill / "SKILL.md"
    meta = _frontmatter(path)
    assert meta["name"] == skill
    assert 50 < len(meta["description"]) <= 1024                  # Agent Skills 规范：description 不超过 1024 字符
    body = path.read_text(encoding="utf-8")
    for link in re.findall(r"\]\(((?:references|scripts)/[^)#]+)\)", body):
        assert (path.parent / link).is_file(), f"{skill} 引用了不存在的文件 {link}"
    for rel in re.findall(r"\$\{CLAUDE_SKILL_DIR\}/([\w./-]+)", body):
        assert (path.parent / rel).is_file(), f"{skill} 的命令指向不存在的文件 {rel}"


def test_agent_builder_points_back_to_agent_prd_and_not_the_reverse_for_code():
    prd = (SKILLS / "agent-prd" / "SKILL.md").read_text(encoding="utf-8")
    builder = (SKILLS / "agent-builder" / "SKILL.md").read_text(encoding="utf-8")
    assert "agent-builder" in prd and "不写代码" in prd
    assert "agent-prd" in builder


def test_bundled_selection_matches_engine_rules():
    mod = _load_script()
    rng = random.Random(7)
    values = ["yes", "no", "later", "unknown", "bogus"]
    cases = [{}, {f.id: {"value": "yes", "evidence": ""} for f in FEATURES},
             {f.id: {"value": "later"} for f in FEATURES}]
    for _ in range(200):
        cases.append({f.id: {"value": rng.choice(values), "evidence": rng.choice(["", "用户说要发消息"])}
                      for f in FEATURES if rng.random() < 0.8})
    for features in cases:
        ours = mod.select_components(features)
        engine = [d.to_dict() for d in select_components(features)]
        assert ours == engine


def test_bundled_tables_match_engine_knowledge():
    mod = _load_script()
    assert mod.FEATURES == [{"id": f.id, "question": f.question, "component": f.component} for f in FEATURES]
    assert mod.COMPONENTS == [{"id": c.id, "name": c.name, "when": c.when, "always": c.always} for c in COMPONENTS]


def test_components_reference_covers_engine_knowledge():
    text = (SKILLS / "agent-prd" / "references" / "components.md").read_text(encoding="utf-8")
    for c in COMPONENTS:
        assert f"| {c.id} {c.name} |" in text and c.when in text and c.production_points in text, c.id
    for f in FEATURES:
        assert f"`{f.id}`" in text and f.question.replace("|", "／") in text, f.id
    for num, name, q, detail in LAYERS:
        assert f"{num} {name}" in text and detail in text
    for key in FACT_KEYS:
        assert f"`{key}`" in text
    for key in FIVE_ELEMENTS:
        assert f"`{key}`" in text
    for name, desc in ACCEPTANCE.values():
        assert name in text and desc in text


def test_script_runs_standalone_from_stdin():
    """插件里没有 blueprint 包：脚本必须只靠标准库、从任意目录跑通。"""
    features = {"destructive_ops": {"value": "yes", "evidence": "替用户发消息"},
                "audit_needed": {"value": "yes"}, "large_knowledge": {"value": "yes"}, "s03": {"value": "yes"}}
    run = subprocess.run([sys.executable, "-I", str(SCRIPT), "-", "--json"], input=json.dumps(features),
                         capture_output=True, text=True, cwd=str(SCRIPT.parent.parent.parent.parent), timeout=30)
    assert run.returncode == 0, run.stderr
    decisions = {d["id"]: d["decision"] for d in json.loads(run.stdout)}
    assert decisions["s03"] == "必选" and decisions["s15"] == "必选" and decisions["s12"] == "待确认"
    assert "s03" in run.stderr                                  # 用组件编号当键会被点名提醒
    table = subprocess.run([sys.executable, "-I", str(SCRIPT), "-"], input="{}", capture_output=True, text=True,
                           timeout=30).stdout
    assert table.startswith("| 组件 | 选否 | 理由 |") and table.count("\n| s") == 17


def test_old_skill_location_is_gone_and_docs_point_to_plugin():
    assert not (ROOT / "skills" / "agent-builder").exists()
    guide = (ROOT / "使用引导.md").read_text(encoding="utf-8")
    assert "plugins/agent-workshop/skills/agent-builder/scripts/init_agent.py" in guide
    assert "skills/agent-builder/" not in guide.replace("plugins/agent-workshop/skills/agent-builder/", "")
