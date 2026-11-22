"""默认模型、依赖清单等配置只允许有一个事实来源；此处守住它们不再漂移。"""
import re
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[1]


def _env_value(path: Path, key: str) -> str:
    for line in path.read_text(encoding="utf-8").splitlines():
        name, sep, value = line.partition("=")
        if sep and name.strip() == key:
            return value.strip().strip('"\'')
    raise AssertionError(f"{path} 缺少 {key}")


def test_default_model_is_consistent_everywhere():
    from blueprint.llm import DEFAULT_MODEL
    from common.bootstrap import DEFAULT_MODEL as DEMO_DEFAULT

    assert DEMO_DEFAULT == DEFAULT_MODEL
    assert _env_value(ROOT / ".env.example", "MODEL_ID") == DEFAULT_MODEL
    assert _env_value(ROOT / "scaffold/.env.example", "MODEL_ID") == DEFAULT_MODEL

    agent = (ROOT / "scaffold/agent.py").read_text(encoding="utf-8")
    m = re.search(r'"model":\s*\("MODEL_ID",\s*str,\s*"([^"]+)"\)', agent)
    assert m and m.group(1) == DEFAULT_MODEL, "scaffold/agent.py 的默认模型与 DEFAULT_MODEL 不一致"

    config = (ROOT / "scaffold/config.example.yaml").read_text(encoding="utf-8")
    m = re.search(r"^model:\s*(\S+)", config, re.M)
    assert m and m.group(1) == DEFAULT_MODEL

    from blueprint.providers import PROVIDER_BY_KEY
    assert PROVIDER_BY_KEY["anthropic"]["model"] == DEFAULT_MODEL, \
        "blueprint/providers.py 的 Anthropic 默认模型不一致（向导和网页界面共用）"


def test_requirements_include_pytest_and_ci_does_not_reinstall_it():
    reqs = (ROOT / "requirements.txt").read_text(encoding="utf-8")
    assert re.search(r"^pytest==", reqs, re.M), "pytest 应锁定在 requirements.txt 中"
    ci_path = ROOT / ".github/workflows/test.yml"
    if not ci_path.is_file():
        pytest.skip("学生包不带 CI 配置")
    assert "requirements.txt pytest" not in ci_path.read_text(encoding="utf-8")


def test_student_package_ships_common_bootstrap():
    import importlib.util
    spec = importlib.util.spec_from_file_location("package_student", ROOT / "tools/package_student.py")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    names = {p.relative_to(ROOT).as_posix() for p in mod.collect_files(ROOT)}
    assert "common/bootstrap.py" in names
    assert "common/__init__.py" in names
    # 测试要和 conftest 一起发：没有它，填过 Key 的使用者跑包里的测试会读到自己的 .env，几条测试因此失败
    assert "tests/conftest.py" in names


def _load_package_student():
    import importlib.util
    spec = importlib.util.spec_from_file_location("package_student", ROOT / "tools/package_student.py")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def test_student_package_can_ship_the_desktop_app(tmp_path):
    """桌面版放在包的最外层、和启动文件挨着；可执行文件解压后还能运行（权限位要带上）。"""
    import stat
    import zipfile
    mod = _load_package_student()
    app = tmp_path / "Agent 工坊.app"
    (app / "Contents" / "MacOS").mkdir(parents=True)
    (app / "Contents" / "Info.plist").write_text("<plist/>", encoding="utf-8")
    exe = app / "Contents" / "MacOS" / "Agent 工坊"
    exe.write_bytes(b"\xcf\xfa\xed\xfe")
    exe.chmod(0o755)
    out = tmp_path / "web.zip"
    mod.package(ROOT, out, app)
    with zipfile.ZipFile(out) as z:
        names = set(z.namelist())
        info = z.getinfo("agent-workshop/Agent 工坊.app/Contents/MacOS/Agent 工坊")
    assert "agent-workshop/启动界面.command" in names
    assert "agent-workshop/Agent 工坊.app/Contents/Info.plist" in names
    assert (info.external_attr >> 16) & stat.S_IXUSR


def test_desktop_app_must_be_a_real_bundle_without_symlinks(tmp_path):
    import pytest
    mod = _load_package_student()
    with pytest.raises(ValueError):
        mod.desktop_app_files(tmp_path / "missing.app")
    app = tmp_path / "Agent 工坊.app"
    (app / "Contents" / "MacOS").mkdir(parents=True)
    (app / "Contents" / "Info.plist").write_text("<plist/>", encoding="utf-8")
    with pytest.raises(ValueError, match="可执行文件"):
        mod.desktop_app_files(app)
    exe = app / "Contents" / "MacOS" / "run"
    exe.write_bytes(b"x")
    exe.chmod(0o755)
    (app / "Contents" / "link").symlink_to(exe)
    with pytest.raises(ValueError, match="符号链接"):
        mod.desktop_app_files(app)
