#!/usr/bin/env python3
"""
Benchmark Results Visualization Script
Creates comprehensive charts from submodel repository benchmark JSON data.

Usage:
    python plot_submodel_benchmark.py <input_json_file> [options]

Examples:
    python plot_submodel_benchmark.py submodelrepo_bench.json
    python plot_submodel_benchmark.py submodelrepo_bench.json -o charts/
    python plot_submodel_benchmark.py submodelrepo_bench.json --unit us
"""
import argparse
import json
from pathlib import Path
from typing import Dict, List, Any
import matplotlib.pyplot as plt
import matplotlib.ticker as ticker
import numpy as np


def load_records(path: Path) -> List[Dict[str, Any]]:
    """Load benchmark records from JSON file."""
    with path.open("r", encoding="utf-8") as f:
        data = json.load(f)
    if isinstance(data, dict) and "records" in data:
        data = data["records"]
    if not isinstance(data, list):
        raise ValueError("Expected a JSON array of log records.")
    return data


def convert_duration(dur_ns: float, dur_ms: float, unit: str) -> float:
    """Convert duration to the specified unit."""
    if dur_ns is not None:
        val = float(dur_ns)
        if unit == "us":
            return val / 1_000.0
        elif unit == "ms":
            return val / 1_000_000.0
        elif unit == "s":
            return val / 1_000_000_000.0
        return val
    else:
        val = float(dur_ms) * 1_000_000.0
        if unit == "us":
            return val / 1_000.0
        elif unit == "ms":
            return val / 1_000_000.0
        elif unit == "s":
            return val / 1_000_000_000.0
        return val


def compute_stats(points: List[tuple]) -> Dict[str, float]:
    """Compute statistical metrics for a series of points."""
    if not points:
        return {"avg": None, "median": None, "p95": None, "p99": None, "trend_pct": None}
    
    xs = [p[0] for p in points]
    ys = [p[1] for p in points]
    
    avg = np.mean(ys)
    median = np.median(ys)
    p95 = np.percentile(ys, 95)
    p99 = np.percentile(ys, 99)
    
    trend_pct = None
    if len(xs) >= 2:
        mean_x = np.mean(xs)
        mean_y = np.mean(ys)
        var_x = np.sum((np.array(xs) - mean_x) ** 2)
        if var_x > 0:
            cov_xy = np.sum((np.array(xs) - mean_x) * (np.array(ys) - mean_y))
            slope = cov_xy / var_x
            if avg > 0:
                trend_pct = (slope / avg) * 100.0
    
    return {
        "avg": avg,
        "median": median,
        "p95": p95,
        "p99": p99,
        "trend_pct": trend_pct
    }


def plot_cumulative_runtime(records: List[Dict], unit: str, output_dir: Path):
    """Create cumulative runtime plot by operation."""
    iters = sorted({r.get("iter") for r in records if "iter" in r})
    ops = sorted({r.get("op", "unknown") for r in records})
    
    if not iters:
        raise ValueError("No records with 'iter' found.")
    
    dur_map = {}
    for r in records:
        it = r.get("iter")
        op = r.get("op", "unknown")
        dur_ns = r.get("duration_ns")
        dur_ms = r.get("duration_ms")
        if it is None or (dur_ns is None and dur_ms is None):
            continue
        
        val = convert_duration(dur_ns, dur_ms, unit)
        dur_map[(it, op)] = dur_map.get((it, op), 0.0) + val
    
    series = {op: [] for op in ops}
    cumulative = {op: 0.0 for op in ops}
    for it in iters:
        for op in ops:
            cumulative[op] += dur_map.get((it, op), 0.0)
            series[op].append(cumulative[op])
    
    per_op_points = {op: [] for op in ops}
    for (it, op), val in dur_map.items():
        if val > 0:
            per_op_points[op].append((it, val))
    
    unit_labels = {"ns": "ns", "us": "µs", "ms": "ms", "s": "s"}
    
    plt.figure(figsize=(12, 7))
    for op in ops:
        stats = compute_stats(sorted(per_op_points[op]))
        avg = stats["avg"]
        trend = stats["trend_pct"]
        
        label = op
        if avg is not None:
            label += f" (avg {avg:.3f} {unit_labels[unit]}"
            if trend is not None:
                label += f", trend {trend:+.3f}%/iter"
            else:
                label += ", trend n/a"
            label += ")"
        
        plt.plot(iters, series[op], marker="o", markersize=3, linewidth=1.3, label=label)
    
    plt.xlabel("Iteration", fontsize=12)
    plt.ylabel(f"Cumulative Runtime ({unit_labels[unit]})", fontsize=12)
    plt.yscale("log")
    plt.title("Cumulative Submodel Repository Benchmark Runtime by Operation", fontsize=14)
    plt.grid(True, linestyle="--", linewidth=0.5, alpha=0.6)
    plt.legend(title="Operation", fontsize=9)
    plt.tight_layout()
    
    output_file = output_dir / "01_cumulative_runtime.png"
    plt.savefig(output_file, dpi=150)
    print(f"✓ Saved: {output_file}")
    plt.close()


def plot_latency_distribution(records: List[Dict], unit: str, output_dir: Path):
    """Create latency distribution histogram by operation."""
    ops = sorted({r.get("op", "unknown") for r in records})
    unit_labels = {"ns": "ns", "us": "µs", "ms": "ms", "s": "s"}
    
    fig, axes = plt.subplots(1, len(ops), figsize=(6 * len(ops), 5), squeeze=False)
    axes = axes.flatten()
    
    for idx, op in enumerate(ops):
        op_records = [r for r in records if r.get("op") == op]
        durations = []
        for r in op_records:
            dur_ns = r.get("duration_ns")
            dur_ms = r.get("duration_ms")
            if dur_ns is not None or dur_ms is not None:
                durations.append(convert_duration(dur_ns, dur_ms, unit))
        
        if durations:
            axes[idx].hist(durations, bins=50, color="steelblue", alpha=0.7, edgecolor="black")
            axes[idx].set_xlabel(f"Latency ({unit_labels[unit]})", fontsize=11)
            axes[idx].set_ylabel("Frequency", fontsize=11)
            axes[idx].set_title(f"Latency Distribution - {op.upper()}", fontsize=12)
            axes[idx].grid(True, linestyle="--", alpha=0.4)
            
            stats = compute_stats([(i, d) for i, d in enumerate(durations)])
            textstr = f"Avg: {stats['avg']:.2f}\nMedian: {stats['median']:.2f}\nP95: {stats['p95']:.2f}\nP99: {stats['p99']:.2f}"
            axes[idx].text(0.65, 0.95, textstr, transform=axes[idx].transAxes, fontsize=9,
                          verticalalignment='top', bbox=dict(boxstyle='round', facecolor='wheat', alpha=0.5))
    
    plt.tight_layout()
    output_file = output_dir / "02_latency_distribution.png"
    plt.savefig(output_file, dpi=150)
    print(f"✓ Saved: {output_file}")
    plt.close()


def plot_throughput_over_time(records: List[Dict], unit: str, output_dir: Path):
    """Create throughput over time plot."""
    iters = sorted({r.get("iter") for r in records if "iter" in r})
    if not iters:
        return
    
    window_size = max(10, len(iters) // 50)
    
    ops = sorted({r.get("op", "unknown") for r in records})
    
    plt.figure(figsize=(12, 7))
    
    for op in ops:
        op_records = [(r.get("iter"), convert_duration(r.get("duration_ns"), r.get("duration_ms"), "s"))
                     for r in records if r.get("op") == op and r.get("iter") is not None]
        op_records.sort()
        
        if len(op_records) < window_size:
            continue
        
        throughputs = []
        iter_points = []
        
        for i in range(len(op_records) - window_size + 1):
            window = op_records[i:i + window_size]
            window_iters = [w[0] for w in window]
            window_durs = [w[1] for w in window]
            total_time = sum(window_durs)
            if total_time > 0:
                throughput = window_size / total_time
                throughputs.append(throughput)
                iter_points.append(window_iters[-1])
        
        if throughputs:
            plt.plot(iter_points, throughputs, marker=".", markersize=2, linewidth=1.2, label=f"{op}")
    
    plt.xlabel("Iteration", fontsize=12)
    plt.ylabel("Throughput (ops/sec)", fontsize=12)
    plt.title(f"Throughput Over Time (rolling window: {window_size} ops)", fontsize=14)
    plt.grid(True, linestyle="--", linewidth=0.5, alpha=0.6)
    plt.legend(title="Operation", fontsize=10)
    plt.tight_layout()
    
    output_file = output_dir / "03_throughput_over_time.png"
    plt.savefig(output_file, dpi=150)
    print(f"✓ Saved: {output_file}")
    plt.close()


def plot_success_rate(records: List[Dict], output_dir: Path):
    """Create success rate plot by operation."""
    ops = sorted({r.get("op", "unknown") for r in records})
    
    success_counts = {op: 0 for op in ops}
    total_counts = {op: 0 for op in ops}
    
    for r in records:
        op = r.get("op", "unknown")
        if op in ops:
            total_counts[op] += 1
            if r.get("ok", False):
                success_counts[op] += 1
    
    success_rates = [(op, 100.0 * success_counts[op] / total_counts[op] if total_counts[op] > 0 else 0)
                    for op in ops]
    
    labels = [sr[0] for sr in success_rates]
    rates = [sr[1] for sr in success_rates]
    
    plt.figure(figsize=(8, 6))
    bars = plt.bar(labels, rates, color=["green" if r >= 95 else "orange" if r >= 80 else "red" for r in rates],
                   alpha=0.7, edgecolor="black")
    
    for bar, rate in zip(bars, rates):
        height = bar.get_height()
        plt.text(bar.get_x() + bar.get_width() / 2., height + 1,
                f'{rate:.1f}%', ha='center', va='bottom', fontsize=11)
    
    plt.xlabel("Operation", fontsize=12)
    plt.ylabel("Success Rate (%)", fontsize=12)
    plt.title("Success Rate by Operation", fontsize=14)
    plt.ylim(0, 110)
    plt.grid(True, axis='y', linestyle="--", alpha=0.4)
    plt.tight_layout()
    
    output_file = output_dir / "04_success_rate.png"
    plt.savefig(output_file, dpi=150)
    print(f"✓ Saved: {output_file}")
    plt.close()


def plot_percentile_comparison(records: List[Dict], unit: str, output_dir: Path):
    """Create percentile comparison bar chart."""
    ops = sorted({r.get("op", "unknown") for r in records})
    unit_labels = {"ns": "ns", "us": "µs", "ms": "ms", "s": "s"}
    
    percentile_data = {}
    for op in ops:
        op_records = [r for r in records if r.get("op") == op]
        durations = []
        for r in op_records:
            dur_ns = r.get("duration_ns")
            dur_ms = r.get("duration_ms")
            if dur_ns is not None or dur_ms is not None:
                durations.append(convert_duration(dur_ns, dur_ms, unit))
        
        if durations:
            percentile_data[op] = {
                "p50": np.percentile(durations, 50),
                "p95": np.percentile(durations, 95),
                "p99": np.percentile(durations, 99),
            }
    
    x = np.arange(len(ops))
    width = 0.25
    
    plt.figure(figsize=(10, 6))
    
    p50_vals = [percentile_data[op]["p50"] for op in ops]
    p95_vals = [percentile_data[op]["p95"] for op in ops]
    p99_vals = [percentile_data[op]["p99"] for op in ops]
    
    plt.bar(x - width, p50_vals, width, label='P50 (Median)', color='skyblue', edgecolor='black')
    plt.bar(x, p95_vals, width, label='P95', color='orange', edgecolor='black')
    plt.bar(x + width, p99_vals, width, label='P99', color='red', edgecolor='black', alpha=0.7)
    
    plt.xlabel("Operation", fontsize=12)
    plt.ylabel(f"Latency ({unit_labels[unit]})", fontsize=12)
    plt.title("Latency Percentile Comparison", fontsize=14)
    plt.xticks(x, ops)
    plt.legend(fontsize=10)
    plt.grid(True, axis='y', linestyle="--", alpha=0.4)
    plt.tight_layout()
    
    output_file = output_dir / "05_percentile_comparison.png"
    plt.savefig(output_file, dpi=150)
    print(f"✓ Saved: {output_file}")
    plt.close()


def generate_summary_report(records: List[Dict], unit: str, output_dir: Path):
    """Generate a text summary report."""
    ops = sorted({r.get("op", "unknown") for r in records})
    unit_labels = {"ns": "ns", "us": "µs", "ms": "ms", "s": "s"}
    
    report_lines = []
    report_lines.append("=" * 80)
    report_lines.append("SUBMODEL REPOSITORY BENCHMARK SUMMARY REPORT")
    report_lines.append("=" * 80)
    report_lines.append(f"Total Records: {len(records)}")
    report_lines.append(f"Unit: {unit_labels[unit]}")
    report_lines.append("")
    
    for op in ops:
        op_records = [r for r in records if r.get("op") == op]
        durations = []
        for r in op_records:
            dur_ns = r.get("duration_ns")
            dur_ms = r.get("duration_ms")
            if dur_ns is not None or dur_ms is not None:
                durations.append(convert_duration(dur_ns, dur_ms, unit))
        
        success_count = sum(1 for r in op_records if r.get("ok", False))
        total_count = len(op_records)
        success_rate = 100.0 * success_count / total_count if total_count > 0 else 0
        
        report_lines.append(f"Operation: {op.upper()}")
        report_lines.append("-" * 40)
        report_lines.append(f"  Total Operations: {total_count}")
        report_lines.append(f"  Success Rate: {success_rate:.2f}% ({success_count}/{total_count})")
        
        if durations:
            stats = compute_stats([(i, d) for i, d in enumerate(durations)])
            report_lines.append(f"  Average Latency: {stats['avg']:.3f} {unit_labels[unit]}")
            report_lines.append(f"  Median Latency: {stats['median']:.3f} {unit_labels[unit]}")
            report_lines.append(f"  P95 Latency: {stats['p95']:.3f} {unit_labels[unit]}")
            report_lines.append(f"  P99 Latency: {stats['p99']:.3f} {unit_labels[unit]}")
            if stats['trend_pct'] is not None:
                report_lines.append(f"  Trend: {stats['trend_pct']:+.3f}%/iteration")
        report_lines.append("")
    
    report_lines.append("=" * 80)
    
    report_text = "\n".join(report_lines)
    
    output_file = output_dir / "00_summary_report.txt"
    with open(output_file, "w") as f:
        f.write(report_text)
    
    print(f"✓ Saved: {output_file}")
    print("\n" + report_text)


def main():
    parser = argparse.ArgumentParser(
        description="Plot submodel repository benchmark results with comprehensive charts.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__
    )
    parser.add_argument("input", type=Path, help="Path to JSON benchmark results file.")
    parser.add_argument("-o", "--output", type=Path, default=None,
                        help="Output directory for charts (default: same directory as input).")
    parser.add_argument("--unit", choices=["ns", "us", "ms", "s"], default="ms",
                        help="Time unit for charts (default: ms).")
    args = parser.parse_args()
    
    if not args.input.exists():
        raise FileNotFoundError(f"Input file not found: {args.input}")
    
    output_dir = args.output if args.output else args.input.parent
    output_dir.mkdir(parents=True, exist_ok=True)
    
    print(f"Loading benchmark data from {args.input}...")
    records = load_records(args.input)
    print(f"Loaded {len(records)} records.\n")
    
    print("Generating charts...")
    generate_summary_report(records, args.unit, output_dir)
    plot_cumulative_runtime(records, args.unit, output_dir)
    plot_latency_distribution(records, args.unit, output_dir)
    plot_throughput_over_time(records, args.unit, output_dir)
    plot_success_rate(records, output_dir)
    plot_percentile_comparison(records, args.unit, output_dir)
    
    print(f"\n✓ All charts generated successfully in: {output_dir}")


if __name__ == "__main__":
    main()
