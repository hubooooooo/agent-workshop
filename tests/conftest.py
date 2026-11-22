"""全部测试共用的设置。"""

import pytest


@pytest.fixture(autouse=True)
def _no_local_env_file(tmp_path, monkeypatch):
    """测试不读本机的 .env：那里填了真实 Key 时，模型配置以 .env 为准，会盖过测试自己设的环境变量。
    要测 .env 的测试自己把 llm.ENV_FILE 指到临时文件。"""
    from blueprint import llm
    monkeypatch.setattr(llm, "ENV_FILE", tmp_path / "no-such-dir" / ".env")
