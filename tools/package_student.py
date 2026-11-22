"""按允许清单生成学生文件包，不收集本地环境、密钥或运行产物。"""
from __future__ import annotations

import argparse
import os
import tempfile
from pathlib import Path
from zipfile import ZIP_DEFLATED, ZipFile

ROOT = Path(__file__).resolve().parents[1]
ROOT_FILES = (
    "README.md", "CONTRIBUTING.md", "使用引导.md", "BLUEPRINT.md", "LICENSE",
    "requirements.txt", "setup.sh", ".env.example", ".gitignore",
    "启动引擎.command", "启动界面.command",
)
PATTERNS = (
    ".vscode/*.json",
    "docs/beginner-guide.md", "docs/prd-toy-theater.md",
    "docs/维护手册.md", "docs/requirements.md", "docs/examples/*.md", "docs/images/*.png",
    "blueprint/*.py", "blueprint/knowledge/*.py", "blueprint/knowledge/prompts/*.md",
    "blueprint/web/*.py", "blueprint/web/static/index.html", "blueprint/web/static/assets/*",
    "common/*.py",
    "s[0-9][0-9]_*/README.md", "s[0-9][0-9]_*/code.py", "s[0-9][0-9]_*/images/*.svg",
    "s[0-9][0-9]_*/s[0-9][0-9]_parts/p[0-9][0-9]_*.py",
    "scaffold/*.py", "scaffold/*.md", "scaffold/*.yaml", "scaffold/requirements.txt", "scaffold/.env.example",
    "skills/*/SKILL.md", "skills/*/references/*.md", "skills/*/references/*.py", "skills/*/scripts/*.py",
    # Claude Code 插件：agent-prd（点子 → PRD 与技术方案）、agent-builder（按方案开发）
    ".claude-plugin/marketplace.json", "plugins/*/.claude-plugin/plugin.json", "plugins/*/README.md",
    "plugins/*/skills/*/SKILL.md", "plugins/*/skills/*/README.md", "plugins/*/skills/*/references/*.md",
    "plugins/*/skills/*/references/*.py", "plugins/*/skills/*/scripts/*.py",
    "evals/README.md", "evals/forward/*/input.md", "evals/forward/*/expected.json",
    "evals/reverse/*/expected.json", "evals/reverse/*/target.txt",
    "evals/reverse/*/README.md", "evals/reverse/*/src/*.py",
    "tests/test_*.py", "tests/conftest.py", "tools/package_student.py", "tools/package_skill.py", "tools/wizard.py", "tools/mac_env.sh", "tools/refresh_examples.py",
    "tools/probe_tools.py",
)


def collect_files(root: Path) -> list[Path]:
    selected = {root / name for name in ROOT_FILES}
    for pattern in PATTERNS:
        selected.update(root.glob(pattern))
    for path in selected:
        relative = path.relative_to(root)
        if not path.is_file():
            raise ValueError(f"缺少分发文件：{relative}")
        if any((root / Path(*relative.parts[:i])).is_symlink()
               for i in range(1, len(relative.parts) + 1)):
            raise ValueError(f"分发文件不能是符号链接：{relative}")
        if path.name == ".env.example":
            # 模板中只能包含占位 Key；不回显任何实际配置值。
            for line in path.read_text(encoding="utf-8").splitlines():
                name, sep, value = line.partition("=")
                if sep and name.strip() in {"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"}:
                    if value.strip().strip('"\'') not in {"", "sk-ant-xxx"}:
                        raise ValueError(f"{relative} 含非占位凭证，请恢复模板再打包")
    return sorted(selected, key=lambda path: path.relative_to(root).as_posix())


SHIPPED_SKILLS = ("agent-prd",)


def skill_archives(root: Path) -> dict[str, bytes]:
    """按 tools/package_skill.py 打出要随学生包分发的技能文件，返回 {包内文件名: 内容}。"""
    import importlib.util
    spec = importlib.util.spec_from_file_location("package_skill", root / "tools" / "package_skill.py")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    out: dict[str, bytes] = {}
    with tempfile.TemporaryDirectory() as tmp:
        for skill in SHIPPED_SKILLS:
            for path in mod.build(skill, Path(tmp)):
                out[path.name] = path.read_bytes()
    return out


def desktop_app_files(app: Path) -> list[Path]:
    """检查要放进包里的桌面版（.app）并列出它的文件。必须是 mygo 打好的应用包，不能有符号链接。"""
    if app.suffix != ".app" or not app.is_dir():
        raise ValueError(f"桌面版要给 .app 文件夹：{app}")
    if not (app / "Contents" / "Info.plist").is_file():
        raise ValueError(f"不是完整的应用包（缺 Contents/Info.plist）：{app}")
    binaries = [p for p in (app / "Contents" / "MacOS").glob("*") if p.is_file() and os.access(p, os.X_OK)]
    if not binaries:
        raise ValueError(f"应用包里没有可执行文件（Contents/MacOS/）：{app}")
    files = []
    for path in sorted(app.rglob("*")):
        if path.is_symlink():
            raise ValueError(f"桌面版里不能有符号链接：{path.relative_to(app)}")
        if path.is_file():
            files.append(path)
    return files


def package(root: Path, output: Path, desktop_app: Path | None = None) -> tuple[int, int]:
    files = collect_files(root)
    app_files = desktop_app_files(desktop_app) if desktop_app else []
    if output.resolve() in {path.resolve() for path in files}:
        raise ValueError("输出路径不能覆盖分发源文件")
    output.parent.mkdir(parents=True, exist_ok=True)
    fd, temporary = tempfile.mkstemp(prefix=".student-", suffix=".zip", dir=output.parent)
    os.close(fd)
    try:
        with ZipFile(temporary, "w", compression=ZIP_DEFLATED, compresslevel=9) as archive:
            for path in files:
                archive.write(path, "agent-workshop/" + path.relative_to(root).as_posix())
            # 打好的 agent-prd：学生用 claude.ai 上传 .skill，用其他 AI 工具就解压 .zip，不用自己打包
            for name, data in skill_archives(root).items():
                archive.writestr("agent-workshop/" + name, data)
            # 桌面版放在最外层，和「启动界面.command」挨着：它向上找引擎目录，第一次打开时自己准备 Python 环境。
            # archive.write 会带上文件权限，可执行文件解压出来还能运行。
            for path in app_files:
                archive.write(path, f"agent-workshop/{desktop_app.name}/" + path.relative_to(desktop_app).as_posix())
        os.replace(temporary, output)
    finally:
        Path(temporary).unlink(missing_ok=True)
    return len(files) + len(app_files), output.stat().st_size


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("-o", "--out", type=Path, default=ROOT / "dist/agent-workshop-web.zip")
    parser.add_argument("--desktop-app", type=Path,
                        help="把打好的桌面版放进包里，如 desktop/dist/darwin-universal/Agent 工坊.app（发版时用通用包）")
    args = parser.parse_args()
    try:
        count, size = package(ROOT, args.out, args.desktop_app)
    except (OSError, ValueError) as exc:
        parser.exit(1, f"打包失败：{exc}\n")
    print(f"已生成：{args.out.resolve()}（{count} 个文件，{size / 1024:.1f} KiB）")
    print(f"   桌面版：{args.desktop_app.name if args.desktop_app else '没有放进去（发版时加 --desktop-app）'}")


if __name__ == "__main__":
    main()
