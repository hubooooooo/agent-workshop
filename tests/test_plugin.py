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


def _load_tool(name):
    import importlib.util
    spec = importlib.util.spec_from_file_location(name, ROOT / "tools" / f"{name}.py")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def test_manual_selection_rules_use_the_scripts_exact_wording():
    """不能跑脚本的 AI 按 components.md 第 7 节手工选型；那里的措辞必须和脚本输出一字不差。"""
    text = (SKILLS / "agent-prd" / "references" / "components.md").read_text(encoding="utf-8")
    manual = text[text.index("## 7. 手工选型规则"):]
    script = SCRIPT.read_text(encoding="utf-8")
    for phrase in ("（一切基础，恒选）", "输入未说明：", "输入表明不需要：", "需归一到单一循环",
                   "二期集成时需要", "可选组件不足 3 个，直接在 loop 里挂接即可",
                   "✅ 必选", "⏸ 二期", "❓ 待确认", "❌ 放弃", "| 组件 | 选否 | 理由 |"):
        assert phrase in manual and phrase in script, phrase
    for value, decision in (("yes", "必选"), ("later", "二期"), ("unknown", "待确认"), ("no", "放弃")):
        assert f'"{value}": "{decision}"' in script
        assert re.search(rf"`{value}`.*\| {decision} \|", manual), value


def test_agent_prd_does_not_depend_on_claude_code_or_agent_builder():
    """学生可能用任何 AI 工具：技能正文不能依赖 Claude Code 才有的变量，也不能假定装了 agent-builder。"""
    body = (SKILLS / "agent-prd" / "SKILL.md").read_text(encoding="utf-8")
    assert "${CLAUDE_" not in body
    assert "没装就把两份文件交给" in body                         # 没装 agent-builder 时的去处
    assert "既不能写文件也不能生成文件" in body                   # 纯聊天环境的交付方式
    meta = _frontmatter(SKILLS / "agent-prd" / "SKILL.md")
    assert meta["license"] == "MIT" and 0 < len(meta["compatibility"]) <= 500


def test_package_skill_builds_identical_skill_and_zip(tmp_path):
    import zipfile
    mod = _load_tool("package_skill")
    first = mod.build("agent-prd", tmp_path / "a")
    again = mod.build("agent-prd", tmp_path / "b")
    assert [p.name for p in first] == ["agent-prd.skill", "agent-prd.zip"]
    assert first[0].read_bytes() == first[1].read_bytes() == again[0].read_bytes()   # 内容相同、可复现
    names = zipfile.ZipFile(first[0]).namelist()
    assert "agent-prd/SKILL.md" in names and "agent-prd/README.md" in names
    assert "agent-prd/scripts/select_components.py" in names
    assert not any("/evals/" in n or "__pycache__" in n for n in names)
    body = (SKILLS / "agent-prd" / "SKILL.md").read_text(encoding="utf-8")
    for link in re.findall(r"\]\(((?:references|scripts)/[^)#]+)\)", body):
        assert f"agent-prd/{link}" in names, link


def test_package_skill_rejects_bad_frontmatter(tmp_path, monkeypatch):
    mod = _load_tool("package_skill")
    bad = tmp_path / "skills" / "bad-skill"
    bad.mkdir(parents=True)
    (bad / "SKILL.md").write_text("---\nname: bad-skill\ndescription: x\nversion: 1\n---\n\n正文\n", encoding="utf-8")
    monkeypatch.setattr(mod, "SKILLS_DIR", tmp_path / "skills")
    with pytest.raises(ValueError, match="规范外的字段"):
        mod.build("bad-skill", tmp_path / "out")


def test_student_package_ships_plugin_skills_and_built_archives():
    """回归：agent-builder 移进 plugins/ 后，学生包只收 skills/，两个技能都漏了。"""
    mod = _load_tool("package_student")
    names = {p.relative_to(ROOT).as_posix() for p in mod.collect_files(ROOT)}
    for skill in ("agent-prd", "agent-builder"):
        skill_dir = SKILLS / skill
        for path in skill_dir.rglob("*"):
            rel = path.relative_to(ROOT).as_posix()
            if path.is_file() and "/evals/" not in rel and "__pycache__" not in rel:
                assert rel in names, rel
    assert ".claude-plugin/marketplace.json" in names
    assert "tools/package_skill.py" in names
    archives = mod.skill_archives(ROOT)
    assert set(archives) == {"agent-prd.skill", "agent-prd.zip"}
