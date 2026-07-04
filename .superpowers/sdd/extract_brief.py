#!/usr/bin/env python3
"""Extract task N from the bedtime plan into a standalone brief file.

Usage: extract_brief.py PLAN_FILE N OUT_FILE
The brief = plan header sections (Global Constraints + Verified UniFi facts)
+ the full text of task N. Prints OUT_FILE on success.
"""
import re
import sys


def main() -> None:
    plan_path, n, out_path = sys.argv[1], int(sys.argv[2]), sys.argv[3]
    text = open(plan_path, encoding="utf-8").read()

    def section(title: str) -> str:
        m = re.search(rf"^## {re.escape(title)}$(.*?)(?=^## )", text, re.M | re.S)
        return f"## {title}{m.group(1)}" if m else ""

    m = re.search(
        rf"^### Task {n}: .*?(?=^### Task \d+: |^## Execution notes)", text, re.M | re.S
    )
    if not m:
        sys.exit(f"task {n} not found in {plan_path}")

    brief = "\n\n".join(
        [
            f"# Task {n} brief — Bedtime implementation plan",
            section("Global Constraints"),
            section("Verified UniFi v2 API facts (probed live 2026-07-02 — treat as ground truth)"),
            m.group(0),
        ]
    )
    with open(out_path, "w", encoding="utf-8") as f:
        f.write(brief)
    print(out_path)


if __name__ == "__main__":
    main()
