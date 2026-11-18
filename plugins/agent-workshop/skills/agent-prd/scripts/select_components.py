#!/usr/bin/env python3
"""Agent 组件选型：把 14 个特征的判断（yes / no / later / unknown）换成 17 个组件的取舍。

纯规则、可复现，不调模型、不联网，只用 Python 标准库（3.8+）。
规则和数据与 Agent 工坊引擎的 blueprint/rules.py、blueprint/knowledge 一致（仓库测试会对照两边）。

用法：
  python3 select_components.py --features                 列出 14 个特征问题（先回答它们）
  python3 select_components.py features.json              按判断出选型（Markdown 表格）
  python3 select_components.py features.json --json       同上，输出 JSON
  cat features.json | python3 select_components.py -      从标准输入读

features.json 的格式：
  {"destructive_ops": {"value": "yes", "evidence": "要替用户发消息"}, ...}
value 只能是 yes / no / later / unknown；漏写的特征按 unknown 处理（会变成「待确认」）。
"""
from __future__ import annotations

import json
import sys

DECISIONS = ("必选", "二期", "待确认", "放弃")
_VALUE_TO_DECISION = {"yes": "必选", "later": "二期", "unknown": "待确认", "no": "放弃"}
_MARK = {"必选": "✅ 必选", "二期": "⏸ 二期", "待确认": "❓ 待确认", "放弃": "❌ 放弃"}

FEATURES = [
    {
        "id": "destructive_ops",
        "question": "是否涉及破坏性或敏感操作（删除、写入外部系统、支付、发送消息、隐私数据）？",
        "component": "s03"
    },
    {
        "id": "audit_needed",
        "question": "是否需要审计、拦截、埋点、合规记录？",
        "component": "s04"
    },
    {
        "id": "multi_step",
        "question": "单次交付是否需要先列计划再逐步推进并跟踪进度（多轮、可中断、要汇报进展）？固定顺序调用几个工具不算。",
        "component": "s05"
    },
    {
        "id": "context_heavy",
        "question": "是否会出现上下文爆炸的子任务，或可并行的探索型子任务？",
        "component": "s06"
    },
    {
        "id": "large_knowledge",
        "question": "领域知识库是否庞大、需要按需加载？",
        "component": "s07"
    },
    {
        "id": "long_sessions",
        "question": "会话是否很长、工具输出或日志量是否很大？",
        "component": "s08"
    },
    {
        "id": "cross_session_memory",
        "question": "是否需要跨会话记住用户偏好、决策、历史？",
        "component": "s09"
    },
    {
        "id": "persistent_goals",
        "question": "目标是否需要持久化、支持断点续跑？",
        "component": "s10"
    },
    {
        "id": "slow_operations",
        "question": "是否有耗时几分钟以上的慢操作需要不阻塞主流程？",
        "component": "s11"
    },
    {
        "id": "scheduled",
        "question": "是否需要定时自动触发？",
        "component": "s12"
    },
    {
        "id": "parallel_isolated",
        "question": "是否需要多任务并行且各自隔离工作区（多 Agent 团队）？",
        "component": "s13"
    },
    {
        "id": "external_ecosystem",
        "question": "是否需要接入外部工具生态（MCP、第三方插件）？",
        "component": "s14"
    },
    {
        "id": "fixed_orchestration",
        "question": "输入是否把流程写成固定的阶段序列（如「上传→翻译→标注→入库」）且各阶段无需模型临场决策？只描述产品体验、或流程里有模型判断分支的不算。",
        "component": "s16"
    },
    {
        "id": "auto_completion",
        "question": "是否需要自动判断「何时算完成」（独立评估器）？",
        "component": "s17"
    }
]

COMPONENTS = [
    {
        "id": "s01",
        "name": "Agent Loop",
        "when": "一切基础：循环 + 工具",
        "always": True
    },
    {
        "id": "s02",
        "name": "工具系统",
        "when": "需要调用外部系统/API/数据源",
        "always": True
    },
    {
        "id": "s03",
        "name": "权限系统",
        "when": "涉及破坏性/敏感操作",
        "always": False
    },
    {
        "id": "s04",
        "name": "钩子系统",
        "when": "需要审计、拦截、埋点",
        "always": False
    },
    {
        "id": "s05",
        "name": "任务规划",
        "when": "多步骤、需跟踪进度",
        "always": False
    },
    {
        "id": "s06",
        "name": "子 Agent",
        "when": "上下文会爆、任务可并行",
        "always": False
    },
    {
        "id": "s07",
        "name": "技能加载",
        "when": "领域知识库庞大",
        "always": False
    },
    {
        "id": "s08",
        "name": "上下文压缩",
        "when": "长会话、日志量大",
        "always": False
    },
    {
        "id": "s09",
        "name": "记忆系统",
        "when": "需跨会话记住偏好/决策",
        "always": False
    },
    {
        "id": "s10",
        "name": "任务系统",
        "when": "目标需持久化、断点续跑",
        "always": False
    },
    {
        "id": "s11",
        "name": "后台任务",
        "when": "有慢操作需不阻塞",
        "always": False
    },
    {
        "id": "s12",
        "name": "定时调度",
        "when": "需定时自治触发",
        "always": False
    },
    {
        "id": "s13",
        "name": "Agent 团队",
        "when": "多任务并行、需隔离工作区",
        "always": False
    },
    {
        "id": "s14",
        "name": "MCP 插件",
        "when": "需接入外部工具生态",
        "always": False
    },
    {
        "id": "s15",
        "name": "集成 Harness",
        "when": "上述机制需协同（选了 3 个以上可选组件）",
        "always": False
    },
    {
        "id": "s16",
        "name": "工作流运行时",
        "when": "编排形态固定",
        "always": False
    },
    {
        "id": "s17",
        "name": "目标闭环",
        "when": "需自动判断「何时算完成」",
        "always": False
    }
]

COMPONENT_BY_ID = {c["id"]: c for c in COMPONENTS}


def select_components(features: dict) -> list:
    """features: {feature_id: {"value": yes|no|later|unknown, "evidence": str}} → 按组件编号排好的决策列表。"""
    decisions = []
    for comp in COMPONENTS:
        if comp["always"]:
            decisions.append({"id": comp["id"], "name": comp["name"], "decision": "必选",
                              "reason": comp["when"] + "（一切基础，恒选）", "feature": ""})
    for feat in FEATURES:
        comp = COMPONENT_BY_ID[feat["component"]]
        raw = features.get(feat["id"]) or {}
        if not isinstance(raw, dict):
            raw = {"value": raw}
        value = raw.get("value", "unknown")
        if value not in _VALUE_TO_DECISION:
            value = "unknown"
        evidence = str(raw.get("evidence") or "").strip()
        decision = _VALUE_TO_DECISION[value]
        if decision == "待确认":
            reason = "输入未说明：" + feat["question"]
        elif decision == "放弃":
            reason = evidence or "输入表明不需要：" + comp["when"]
        else:
            reason = evidence or comp["when"]
        decisions.append({"id": comp["id"], "name": comp["name"], "decision": decision,
                          "reason": reason, "feature": feat["id"]})

    # s15 集成 Harness：可选组件里「必选」达到 3 个则必选；连同二期达到 3 个则二期
    optional = [d for d in decisions if d["id"] not in ("s01", "s02", "s15")]
    mvp = [d for d in optional if d["decision"] == "必选"]
    later = [d for d in optional if d["decision"] in ("必选", "二期")]
    name15 = COMPONENT_BY_ID["s15"]["name"]
    if len(mvp) >= 3:
        d15 = ("必选", "第一版已选 {} 个可选组件（{}），需归一到单一循环".format(len(mvp), ", ".join(d["id"] for d in mvp)))
    elif len(later) >= 3:
        d15 = ("二期", "含二期共 {} 个可选组件，二期集成时需要".format(len(later)))
    else:
        d15 = ("放弃", "可选组件不足 3 个，直接在 loop 里挂接即可")
    decisions.append({"id": "s15", "name": name15, "decision": d15[0], "reason": d15[1], "feature": ""})

    order = {c["id"]: i for i, c in enumerate(COMPONENTS)}
    decisions.sort(key=lambda d: order[d["id"]])
    return decisions


def render_markdown(decisions: list) -> str:
    lines = ["| 组件 | 选否 | 理由 |", "|---|---|---|"]
    for d in decisions:
        reason = d["reason"].replace("|", "／").replace("\n", " ")
        lines.append("| {} {} | {} | {} |".format(d["id"], d["name"], _MARK[d["decision"]], reason))
    return "\n".join(lines)


def _features_help() -> str:
    lines = ["回答这 14 个问题（value 填 yes / no / later / unknown，evidence 写一句依据）：", ""]
    for f in FEATURES:
        comp = COMPONENT_BY_ID[f["component"]]
        lines.append("- {}（决定 {} {}）：{}".format(f["id"], comp["id"], comp["name"], f["question"]))
    return "\n".join(lines)


def main(argv: list) -> int:
    args = [a for a in argv if not a.startswith("--")]
    flags = {a for a in argv if a.startswith("--")}
    if "--features" in flags:
        print(_features_help())
        return 0
    if not args:
        print(__doc__.strip())
        return 2
    text = sys.stdin.read() if args[0] == "-" else open(args[0], encoding="utf-8").read()
    try:
        features = json.loads(text)
    except ValueError as exc:
        print("features 不是合法 JSON：{}".format(exc), file=sys.stderr)
        return 1
    if not isinstance(features, dict):
        print("features 必须是 JSON 对象，键是特征 id", file=sys.stderr)
        return 1
    known = {f["id"] for f in FEATURES}
    unknown_keys = sorted(set(features) - known)
    if unknown_keys:
        print("忽略不认识的特征：{}（键要用特征 id，不是组件编号）".format(", ".join(unknown_keys)), file=sys.stderr)
    decisions = select_components(features)
    if "--json" in flags:
        print(json.dumps(decisions, ensure_ascii=False, indent=2))
    else:
        print(render_markdown(decisions))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
