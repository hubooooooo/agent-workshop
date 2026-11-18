"""把插件里的技能打成可分发的文件：给 claude.ai 上传的 .skill，给其他 AI 工具解压用的 .zip。

两个文件内容完全一样：压缩包里是一个以技能名命名的文件夹（SKILL.md、references/、scripts/、README.md），
不含测试用例（evals/）和缓存。格式与 Anthropic skill-creator 的 package_skill 一致。

用法：
  .venv/bin/python tools/package_skill.py                    # 打包 agent-prd 到 dist/
  .venv/bin/python tools/package_skill.py agent-builder -o out/dist
"""
from __future__ import annotations

import argparse
import re
import sys
import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SKILLS_DIR = ROOT / "plugins" / "agent-workshop" / "skills"
EXCLUDE_DIRS = {"__pycache__", "node_modules", ".pytest_cache"}
EXCLUDE_ROOT_DIRS = {"evals"}                 # 测试用例只在仓库里用，不发给学生
EXCLUDE_NAMES = {".DS_Store"}
ALLOWED_KEYS = {"name", "description", "license", "allowed-tools", "metadata", "compatibility"}
# 固定时间戳：同样的内容每次打出的压缩包逐字节相同，便于比对和缓存
FIXED_TIME = (2026, 1, 1, 0, 0, 0)


def frontmatter(skill_md: Path) -> dict[str, str]:
    text = skill_md.read_text(encoding="utf-8")
    m = re.match(r"^---\n(.*?)\n---\n", text, re.S)
    if not m:
        raise ValueError(f"{skill_md} 缺少 YAML frontmatter")
    out: dict[str, str] = {}
    for line in m.group(1).splitlines():
        if not line.strip():
            continue
        key, sep, value = line.partition(":")
        if not sep or line[:1].isspace():
            raise ValueError(f"{skill_md} 的 frontmatter 只支持单行 key: value，这一行不行：{line!r}")
        out[key.strip()] = value.strip()
    return out


def validate(skill_dir: Path) -> dict[str, str]:
    """按 Agent Skills 规范检查 frontmatter；不合格就报错，免得发出去别人装不上。"""
    meta = frontmatter(skill_dir / "SKILL.md")
    extra = set(meta) - ALLOWED_KEYS
    if extra:
        raise ValueError(f"frontmatter 有规范外的字段：{', '.join(sorted(extra))}")
    name, description = meta.get("name", ""), meta.get("description", "")
    if name != skill_dir.name or not re.fullmatch(r"[a-z0-9]+(-[a-z0-9]+)*", name) or len(name) > 64:
        raise ValueError(f"name 必须等于文件夹名、只用小写字母数字和连字符：{name!r}")
    if not description or len(description) > 1024:
        raise ValueError(f"description 不能为空、不能超过 1024 个字符（现在 {len(description)}）")
    if len(meta.get("compatibility", "")) > 500:
        raise ValueError("compatibility 不能超过 500 个字符")
    return meta


def skill_files(skill_dir: Path) -> list[Path]:
    files = []
    for path in sorted(skill_dir.rglob("*")):
        rel = path.relative_to(skill_dir)
        if not path.is_file() or path.is_symlink():
            continue
        if rel.parts[0] in EXCLUDE_ROOT_DIRS or EXCLUDE_DIRS & set(rel.parts) or path.name in EXCLUDE_NAMES:
            continue
        if path.suffix == ".pyc":
            continue
        files.append(path)
    return files


def build(skill: str, out_dir: Path) -> list[Path]:
    skill_dir = SKILLS_DIR / skill
    if not (skill_dir / "SKILL.md").is_file():
        raise ValueError(f"找不到技能：{skill_dir}")
    validate(skill_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    outputs = []
    for ext in (".skill", ".zip"):
        target = out_dir / f"{skill}{ext}"
        with zipfile.ZipFile(target, "w", zipfile.ZIP_DEFLATED) as zf:
            for path in skill_files(skill_dir):
                arcname = f"{skill}/{path.relative_to(skill_dir).as_posix()}"
                info = zipfile.ZipInfo(arcname, FIXED_TIME)
                info.compress_type = zipfile.ZIP_DEFLATED
                info.external_attr = (0o755 if path.suffix == ".py" else 0o644) << 16
                zf.writestr(info, path.read_bytes())
        outputs.append(target)
    return outputs


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="把插件里的技能打成 .skill 和 .zip")
    parser.add_argument("skill", nargs="?", default="agent-prd", help="技能名（plugins/agent-workshop/skills 下的文件夹）")
    parser.add_argument("-o", "--out", default=str(ROOT / "dist"), help="输出目录，默认 dist/")
    args = parser.parse_args(argv)
    try:
        outputs = build(args.skill, Path(args.out))
    except ValueError as exc:
        print(f"⛔ {exc}", file=sys.stderr)
        return 1
    for path in outputs:
        print(f"✅ {path}（{path.stat().st_size // 1024} KB）")
    print("   .skill 给 claude.ai / Claude App 上传；.zip 解压后给其他 AI 工具用。")
    return 0


if __name__ == "__main__":
    sys.exit(main())
