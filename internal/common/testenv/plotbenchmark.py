#!/usr/bin/env python3
import argparse
import json
from pathlib import Path
import matplotlib.pyplot as plt

def load_records(path: Path):
    with path.open("r", encoding="utf-8") as f:
        data = json.load(f)
    if isinstance(data, dict) and "records" in data:
        data = data["records"]
    if not isinstance(data, list):
        raise ValueError("Expected a JSON array of log records.")
    return data

def main():
    parser = argparse.ArgumentParser(description="Plot cumulative discovery benchmark runtimes per op.")
    parser.add_argument("input", type=Path, help="Path to JSON log (array of records).")
    parser.add_argument("-o", "--output", type=Path, default=None,
                        help="Optional output image (e.g., plot.png). If omitted, shows a window.")
    parser.add_argument("--unit", choices=["us", "ms"], default="us",
                        help="Y-axis units. Default: microseconds (us).")
    args = parser.parse_args()

    records = load_records(args.input)

    # Collect all iterations and operations
    iters = sorted({r.get("iter") for r in records if "iter" in r})
    ops = sorted({r.get("op", "unknown") for r in records})

    if not iters:
        raise SystemExit("No records with 'iter' found.")

    # Build (iter, op) -> duration map
    dur_map = {}
    for r in records:
        it = r.get("iter")
        op = r.get("op", "unknown")
        dur = r.get("duration_ms")
        if it is None or dur is None:
            continue

        val = float(dur)  # value in microseconds (true unit)
        if args.unit == "ms":
            val /= 1000.0
        dur_map[(it, op)] = dur_map.get((it, op), 0.0) + val

    # Build cumulative per operation
    series = {op: [] for op in ops}
    cumulative = {op: 0.0 for op in ops}
    for it in iters:
        for op in ops:
            cumulative[op] += dur_map.get((it, op), 0.0)
            series[op].append(cumulative[op])

    y_label = "Cumulative runtime (µs)" if args.unit == "us" else "Cumulative runtime (ms)"

    # Plot
    plt.figure(figsize=(10, 6))
    for op in ops:
        plt.plot(iters, series[op], marker="o", markersize=3, linewidth=1.3, label=op)

    plt.xlabel("Iteration")
    plt.ylabel(y_label)
    plt.title("Cumulative Discovery Benchmark Runtime by Operation")
    plt.grid(True, linestyle="--", linewidth=0.5, alpha=0.6)
    plt.legend(title="Operation")
    plt.tight_layout()

    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        plt.savefig(args.output, dpi=150)
        print(f"Saved plot to {args.output}")
    else:
        plt.show()

if __name__ == "__main__":
    main()
