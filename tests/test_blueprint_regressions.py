"""改造边界回归：缓存、扫描证据、CLI 与评估结果，全部离线。"""
from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT))

from blueprint.forward import run_plan
from blueprint.llm import AnthropicRunner, Budget, has_credentials
from blueprint.mock import MockRunner
from blueprint.reverse import run_audit
from blueprint.scanner import scan_project


def quiet(_message):
    pass


def test_resume_changed_input_does_not_mix_old_plan(tmp_path):
    run_plan("每天定时提醒", MockRunner(), tmp_path, log=quiet)
    runner = MockRunner()
    plan, _ = run_plan("论文阅读助手", runner, tmp_path, resume=True, log=quiet)
    assert runner.calls == ["extract", "model", "design", "review"]
    assert plan.title == "论文阅读助手"
    assert (tmp_path / "input.md").read_text() == "论文阅读助手"


@pytest.mark.parametrize("damage", ['{"facts":{}}', '{"invalid":', '[]'])
def test_resume_repairs_invalid_cache(tmp_path, damage):
    run_plan("一个点子", MockRunner(), tmp_path, log=quiet)
    (tmp_path / "stages/extract.json").write_text(damage)
    runner = MockRunner()
    run_plan("一个点子", runner, tmp_path, resume=True, log=quiet)
    assert "extract" in runner.calls


def test_resume_model_change_invalidates_cache(tmp_path):
    runner = MockRunner()
    runner.model = "one"
    run_plan("一个点子", runner, tmp_path, log=quiet)
    runner = MockRunner()
    runner.model = "two"
    run_plan("一个点子", runner, tmp_path, resume=True, log=quiet)
    assert runner.calls == ["extract", "model", "design", "review"]


def test_audit_resume_detects_source_edit_without_signal_change(tmp_path):
    project = tmp_path / "project"
    project.mkdir()
    source = project / "app.py"
    source.write_text("print('version one')")
    out = tmp_path / "out"
    run_audit(project, MockRunner(), out, log=quiet)
    runner = MockRunner()
    run_audit(project, runner, out, resume=True, log=quiet)
    assert runner.calls == []
    source.write_text("print('version two')")
    runner = MockRunner()
    run_audit(project, runner, out, resume=True, log=quiet)
    assert runner.calls == ["infer", "audit_review"]


def test_docs_and_tests_cannot_establish_runtime_components(tmp_path):
    claims = "Anthropic()\n" + "permission check\n" * 6 + "logger.info('ok')\n"
    (tmp_path / "README.md").write_text(claims)
    (tmp_path / "test_fake.py").write_text(claims)
    inv = scan_project(tmp_path)
    assert inv.components["s03"].strength == "none"
    assert inv.components["s03"].doc_files == 2
    assert not inv.llm_sdks
    assert inv.acceptance["observable"]["status"] == "fail"


def test_scanner_does_not_read_symlink_targets(tmp_path):
    project = tmp_path / "project"
    project.mkdir()
    secret = tmp_path / "external.txt"
    secret.write_text("Anthropic()\npermission check\n")
    for name in ("README.md", "app.py", "requirements.txt"):
        (project / name).symlink_to(secret)
    (project / "loop").symlink_to(project, target_is_directory=True)
    inv = scan_project(project)
    assert not inv.readme and not inv.dependencies and not inv.llm_sdks
    assert inv.file_count == 0


def test_scanner_prunes_excluded_directories(tmp_path, monkeypatch):
    ignored = tmp_path / "node_modules"
    ignored.mkdir()
    (ignored / "signal.py").write_text("Anthropic()")
    import os
    original = os.scandir
    def guarded(path):
        assert Path(path) != ignored, "扫描器不应进入 node_modules"
        return original(path)
    monkeypatch.setattr(os, "scandir", guarded)
    assert scan_project(tmp_path).file_count == 0


def test_cli_long_text_and_empty_input(tmp_path):
    command = [sys.executable, "-m", "blueprint", "plan"]
    result = subprocess.run(command + ["长点子" * 300, "--mock", "-o", str(tmp_path)],
                            cwd=ROOT, capture_output=True, text=True)
    assert result.returncode == 0, result.stderr
    assert (tmp_path / "方案.md").exists()
    result = subprocess.run(command + [" ", "--mock", "-o", str(tmp_path)],
                            cwd=ROOT, capture_output=True, text=True)
    assert result.returncode == 1 and "不能为空" in result.stdout
    assert "Traceback" not in result.stderr


def test_environment_takes_precedence_and_client_settings(monkeypatch):
    monkeypatch.setenv("MODEL_ID", "explicit-model")
    monkeypatch.setenv("ANTHROPIC_API_KEY", "test-key")
    monkeypatch.delenv("ANTHROPIC_AUTH_TOKEN", raising=False)
    import anthropic
    settings = {}
    def fake_client(**kwargs):
        settings.update(kwargs)
        return object()
    monkeypatch.setattr(anthropic, "Anthropic", fake_client)
    runner = AnthropicRunner(budget=Budget())
    assert runner.model == "explicit-model"
    assert settings["timeout"] == 120 and settings["max_retries"] == 0


@pytest.mark.parametrize("key", ["", "sk-ant-xxx", "your-api-key"])
def test_placeholder_credentials_are_not_real(monkeypatch, key):
    monkeypatch.setenv("ANTHROPIC_API_KEY", key)
    monkeypatch.setenv("ANTHROPIC_AUTH_TOKEN", "")
    assert not has_credentials()


def test_real_eval_differences_fail_exit_status(monkeypatch, capsys):
    from blueprint import evals
    import tempfile
    monkeypatch.setattr(evals, "forward_cases", lambda: [ROOT / "evals/forward/paper-reader"])
    monkeypatch.setattr(evals, "reverse_cases", lambda: [])
    with tempfile.TemporaryDirectory() as directory:
        monkeypatch.setattr(evals.tempfile, "mkdtemp", lambda **_: directory)
        assert evals.run_evals(real=True, runner_factory=MockRunner) == 1
    assert "项失败" in capsys.readouterr().out


def test_reports_show_actual_model_not_runner_name(tmp_path):
    """报告里的「模型」要写实际模型（如 deepseek-v4-flash），不能写运行器名 anthropic；离线演示仍写 mock。"""
    class NamedModel(MockRunner):
        model = "deepseek-v4-flash"

    plan, doc = run_plan("论文阅读助手", NamedModel(), tmp_path / "named", log=quiet)
    assert plan.runner == "mock" and plan.model == "deepseek-v4-flash"
    assert "模型：deepseek-v4-flash" in doc.read_text(encoding="utf-8")
    plan, doc = run_plan("论文阅读助手", MockRunner(), tmp_path / "mock", log=quiet)
    assert "模型：mock" in doc.read_text(encoding="utf-8")
    project = tmp_path / "proj"
    project.mkdir()
    (project / "agent.py").write_text("def loop():\n    pass\n", encoding="utf-8")
    audit, doc = run_audit(project, NamedModel(), tmp_path / "audit", log=quiet)
    assert audit.model == "deepseek-v4-flash" and "模型：deepseek-v4-flash" in doc.read_text(encoding="utf-8")


def test_pending_list_groups_match_terminal_summary(tmp_path):
    """待确认清单按来源分组、编号连续；终端提示的条数与清单一致，不再只数需求问题。"""
    from blueprint.render import OPENS_HEADING, pending_groups, pending_summary, render_plan
    plan, _doc = run_plan("每天定时整理邮件并提醒我", MockRunner(), tmp_path, log=quiet)
    plan.design["open_questions"] = ["邮件存哪里？", "要不要支持多个邮箱？"]
    block = render_plan(plan).split("## ⚠️ 待确认清单", 1)[1].split("\n## ", 1)[0]
    needs, opens, comps = pending_groups(plan)
    numbers = [int(l.split(".", 1)[0]) for l in block.splitlines() if l[:1].isdigit()]
    assert numbers == list(range(1, len(needs) + len(opens) + len(comps) + 1)) and OPENS_HEADING in block
    assert "2 个方案留下的问题" in pending_summary(plan)


def test_architecture_diagram_reads_layer_names_and_stays_short():
    """归属层写成「⑤能力集成层」也要进第 ⑤ 层；第 ⑦ 层只放部署说明的第一句；原文里不出现 #38; 这类转义。"""
    from blueprint.render import architecture_nodes, render_mermaid
    tools = [{"name": "search_menu", "layer": "⑤能力集成层"}, {"name": "save_order", "layer": "6 数据层"}]
    nodes = dict(architecture_nodes([], tools, "第一版部署形态：单进程服务。" + "很长的说明" * 50))
    assert nodes["⑤ 能力集成层"] == "search_menu" and nodes["⑥ 数据层"] == "save_order"
    assert nodes["⑦ 基础设施层"] == "第一版部署形态：单进程服务"
    chart = render_mermaid("t", [], tools, {}, "A & B <c>")
    assert "#38;" not in chart and "A ＆ B ＜c＞" in chart


def test_env_file_with_real_key_wins_over_inherited_environment(monkeypatch, tmp_path):
    """回归：课堂上由 AI 编程工具在终端里运行命令，有的工具自己设了 ANTHROPIC_BASE_URL，曾经盖过 .env 里
    DeepSeek 的地址，请求发到别的服务商，报「Key 无效（401）」。.env 有真实 Key 时，模型配置以 .env 为准。"""
    import os
    from blueprint import llm
    env_file = tmp_path / ".env"
    env_file.write_text("ANTHROPIC_API_KEY=sk-real-deepseek\nMODEL_ID=deepseek-v4-flash\n"
                        "ANTHROPIC_BASE_URL=https://api.deepseek.com/anthropic\n", encoding="utf-8")
    monkeypatch.setattr(llm, "ENV_FILE", env_file)
    monkeypatch.setenv("ANTHROPIC_BASE_URL", "https://api.anthropic.com")
    monkeypatch.setenv("ANTHROPIC_API_KEY", "sk-from-the-tool")
    monkeypatch.setenv("ANTHROPIC_AUTH_TOKEN", "token-from-the-tool")
    monkeypatch.setenv("BLUEPRINT_TIMEOUT", "300")
    llm.load_config()
    assert os.environ["ANTHROPIC_BASE_URL"] == "https://api.deepseek.com/anthropic"
    assert os.environ["ANTHROPIC_API_KEY"] == "sk-real-deepseek"
    assert os.environ["MODEL_ID"] == "deepseek-v4-flash"
    assert "ANTHROPIC_AUTH_TOKEN" not in os.environ
    assert os.environ["BLUEPRINT_TIMEOUT"] == "300"


def test_env_file_without_real_key_keeps_environment(monkeypatch, tmp_path):
    """.env 里还是模板的占位 Key 时，照旧用环境变量里的配置。"""
    import os
    from blueprint import llm
    env_file = tmp_path / ".env"
    env_file.write_text("ANTHROPIC_API_KEY=sk-ant-xxx\nMODEL_ID=claude-sonnet-4-6\n", encoding="utf-8")
    monkeypatch.setattr(llm, "ENV_FILE", env_file)
    monkeypatch.setenv("ANTHROPIC_API_KEY", "sk-real-from-shell")
    monkeypatch.setenv("ANTHROPIC_BASE_URL", "https://api.deepseek.com/anthropic")
    llm.load_config()
    assert os.environ["ANTHROPIC_API_KEY"] == "sk-real-from-shell"
    assert os.environ["ANTHROPIC_BASE_URL"] == "https://api.deepseek.com/anthropic"


def test_resume_hint_only_suggests_resume_where_it_exists():
    """只有 plan、audit 收 --resume。回归：init --plan 被打断时曾提示「加 --resume」，照做会直接报参数错误；
    教学版的 revise 失败时需求文档已经升版，重跑 revise 会再升一版，所以提示改用 plan --resume。"""
    from argparse import Namespace
    from blueprint.cli import build_parser, resume_hint
    assert "--resume" in resume_hint(Namespace(cmd="plan"))
    assert "--resume" in resume_hint(Namespace(cmd="audit"))
    hint = resume_hint(Namespace(cmd="init", plan=True, name="酒馆 搭子"))
    assert "blueprint plan" in hint and "酒馆-搭子" in hint and hint.endswith("--resume")
    revise = resume_hint(Namespace(cmd="revise", workspace="out/酒馆搭子"))
    assert "blueprint plan" in revise and "out/酒馆搭子/PRD.md" in revise and "不要重跑 revise" in revise
    assert "--resume" not in resume_hint(Namespace(cmd="verify"))
    build_parser().parse_args(["plan", "out/x/PRD.md", "-o", "out/x", "--resume"])


def test_api_error_prints_a_hint_instead_of_a_traceback(monkeypatch, capsys):
    """回归：教学版接口报错时直接抛出整段堆栈，学生看不懂；现在给一句原因和续跑命令。"""
    import anthropic
    import httpx
    from blueprint import cli

    def failing_plan(args):
        raise anthropic.APIConnectionError(request=httpx.Request("POST", "https://example.invalid/v1/messages"))

    parser = cli.build_parser()
    parser._subparsers._group_actions[0].choices["plan"].set_defaults(func=failing_plan)
    monkeypatch.setattr(cli, "build_parser", lambda: parser)
    with pytest.raises(SystemExit) as exit_info:
        cli.main(["plan", "一句话点子", "-o", "out/x"])
    assert exit_info.value.code == 4
    out = capsys.readouterr().out
    assert "模型接口报错" in out and "原命令加 --resume" in out
