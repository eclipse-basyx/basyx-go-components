#!/usr/bin/env python3
import argparse
import json
from pathlib import Path
import matplotlib.pyplot as plt
import matplotlib.ticker as ticker

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
    parser.add_argument("--unit", choices=["ns", "us", "ms", "s"], default="ms",
                        help="Y-axis units. Default: ms.")
    args = parser.parse_args()

    records = load_records(args.input)

    # Collect all iterations and operations
    iters = sorted({r.get("iter") for r in records if "iter" in r})
    ops = sorted({r.get("op", "unknown") for r in records})

    if not iters:
        raise SystemExit("No records with 'iter' found.")

    # Build (iter, op) -> duration map (in chosen unit)
    dur_map = {}
    for r in records:
        it = r.get("iter")
        op = r.get("op", "unknown")
        dur_ns = r.get("duration_ns")
        dur_ms = r.get("duration_ms")
        if it is None or (dur_ns is None and dur_ms is None):
            continue

        # Convert everything into the chosen unit
        if dur_ns is not None:
            val = float(dur_ns)  # ns
            if args.unit == "us":
                val /= 1_000.0
            elif args.unit == "ms":
                val /= 1_000_000.0
            elif args.unit == "s":
                val /= 1_000_000_000.0
        else:
            # duration_ms is present
            val = float(dur_ms) * 1_000_000.0  # ms -> ns
            if args.unit == "us":
                val /= 1_000.0
            elif args.unit == "ms":
                val /= 1_000_000.0
            elif args.unit == "s":
                val /= 1_000_000_000.0

        dur_map[(it, op)] = dur_map.get((it, op), 0.0) + val

    # Build cumulative per operation (for plotting)
    series = {op: [] for op in ops}
    cumulative = {op: 0.0 for op in ops}
    for it in iters:
        for op in ops:
            cumulative[op] += dur_map.get((it, op), 0.0)
            series[op].append(cumulative[op])

    # Collect per-op raw points for averages and trend (ONLY non-missing points)
    per_op_points = {op: [] for op in ops}  # list of (iter, value) per op
    for (it, op), val in dur_map.items():
        if val > 0:
            per_op_points[op].append((it, val))

    # Compute per-op averages and trend (slope % per iteration)
    avg_by_op = {}
    trend_pct_per_iter_by_op = {}
    unit_labels = {"ns": "ns", "us": "µs", "ms": "ms", "s": "s"}
    y_label = f"Cumulative runtime ({unit_labels[args.unit]})"

    for op in ops:
        pts = sorted(per_op_points[op])  # sort by iter
        if not pts:
            avg_by_op[op] = None
            trend_pct_per_iter_by_op[op] = None
            continue

        xs = [p[0] for p in pts]
        ys = [p[1] for p in pts]

        # Average (over occurrences where the op appears)
        avg = sum(ys) / len(ys)
        avg_by_op[op] = avg

        # Trend via simple least-squares slope; need at least 2 points
        if len(xs) >= 2:
            mean_x = sum(xs) / len(xs)
            mean_y = sum(ys) / len(ys)
            var_x = sum((x - mean_x) ** 2 for x in xs)
            if var_x > 0:
                cov_xy = sum((x - mean_x) * (y - mean_y) for x, y in zip(xs, ys))
                slope = cov_xy / var_x  # units per iteration (same unit as y)
                if avg > 0:
                    trend_pct_per_iter = (slope / avg) * 100.0  # % per iter
                    trend_pct_per_iter_by_op[op] = trend_pct_per_iter
                else:
                    trend_pct_per_iter_by_op[op] = None
            else:
                trend_pct_per_iter_by_op[op] = None
        else:
            trend_pct_per_iter_by_op[op] = None

    # Plot
    plt.figure(figsize=(10, 6))
    for op in ops:
        avg = avg_by_op.get(op)
        trend_pct = trend_pct_per_iter_by_op.get(op)

        label = op
        if avg is not None:
            label += f" (avg {avg:.3f} {unit_labels[args.unit]}"
            if trend_pct is not None:
                # include sign; show % per iteration
                label += f", trend {trend_pct:+.3f}%/iter"
            else:
                label += ", trend n/a"
            label += ")"

        plt.plot(iters, series[op], marker="o", markersize=3, linewidth=1.3, label=label)

    plt.xlabel("Iteration")
    plt.ylabel(y_label)
    plt.yscale("log")
    plt.title("Cumulative Discovery Benchmark Runtime by Operation")
    plt.grid(True, linestyle="--", linewidth=0.5, alpha=0.6)
    plt.legend(title="Operation", fontsize=8)

    # Optional: format large numbers nicely
    def smart_format(x, _):
        if x >= 1e9:
            return f"{x/1e9:.1f}B"
        elif x >= 1e6:
            return f"{x/1e6:.1f}M"
        elif x >= 1e3:
            return f"{x/1e3:.1f}K"
        return f"{x:.0f}"

    plt.gca().yaxis.set_major_formatter(ticker.FuncFormatter(smart_format))
    plt.tight_layout()

    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        plt.savefig(args.output, dpi=150)
        print(f"Saved plot to {args.output}")
    else:
        plt.show()

if __name__ == "__main__":
    main()
