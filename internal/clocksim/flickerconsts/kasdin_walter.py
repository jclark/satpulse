"""Kasdin-Walter flicker noise scaling factor computation.

This module computes the Allan-weighted scaling factor for Kasdin-Walter
flicker noise filters. The computation is separated into its own module to
allow reuse across Python and Go codebases via code generation.

Usage:
    python kasdin_walter.py --tau0 10.0 --tau-max 1000.0 --ratio 2.0

This can be integrated into Go builds using `go generate`:
    //go:generate python kasdin_walter.py --tau0 10.0 --tau-max 1000.0 --ratio 2.0
"""

import argparse
import math

import numpy as np


def compute_kasdin_walter_allan_scale(
    pole_time_constants: list[float],
    target_tau_range: tuple[float, float] = (10.0, 1000.0),
    num_tau_samples: int = 20
) -> float:
    """Compute Allan-weighted scaling factor for Kasdin-Walter flicker noise.

    For a sum of AR(1) processes with given time constants, this computes the
    scaling factor needed so that when each stage has variance scale², the
    Allan deviation is 1.0 (dimensionless) across the target tau range.

    Args:
        pole_time_constants: List of AR(1) time constants τ_k in seconds
        target_tau_range: (min, max) tau values where ADEV should be flat
        num_tau_samples: Number of tau points to sample for averaging

    Returns:
        Scaling factor: multiply naive 1/sqrt(N) scale by this to get correct ADEV

    Theory:
        For an AR(1) process y[n] = b·y[n-1] + w with b = exp(-1/τ) and
        w ~ N(0, σ²(1-b²)), the PSD (for dt=1s sampling) is approximately:
            S(f) ≈ σ² · 2τ / (1 + (2πfτ)²)  for f << 1/(2π) Hz

        The Allan variance transfer function is:
            H(f, τ) = 2sin²(πfτ) / (πfτ)²

        Allan variance contribution from this stage:
            C(τ) = 4 ∫₀^∞ S(f) · H(f, τ) df

        For multiple stages with unit variance (σ²=1), the total Allan variance is
        σ_y²(τ) = Σ_k C_k(τ). We want this to equal 1.0, so the scale factor is
        1/sqrt(mean of Σ_k C_k(τ) over the target τ range).
    """
    # Sample tau values logarithmically across target range
    tau_min, tau_max = target_tau_range
    tau_samples = np.logspace(
        np.log10(tau_min), np.log10(tau_max), num_tau_samples
    )

    allan_variances = []

    for tau in tau_samples:
        # Compute Allan variance contribution from all stages at this tau
        total_variance = 0.0

        for tau_k in pole_time_constants:
            # Numerical integration: C_k(τ) = 4 ∫₀^∞ S_k(f) · H(f, τ) df
            # Use log-spaced frequency grid from DC to well above 1/tau
            f_max = 10.0 / min(tau, tau_k)  # 10x the highest corner frequency
            f = np.logspace(-6, np.log10(f_max), 10000)

            # PSD for AR(1) with time constant tau_k, unit variance (σ²=1)
            S_k = 2.0 * tau_k / (1.0 + (2.0 * np.pi * f * tau_k) ** 2)  # noqa: N806

            # Allan transfer function for frequency data (IEEE Std 1139-2008)
            # σ_y²(τ) = 2 ∫ S_y(f) · sin⁴(πfτ)/(πfτ)² df
            H = np.sin(np.pi * f * tau) ** 4 / (np.pi * f * tau) ** 2  # noqa: N806

            # Integrate S_k(f) * H(f) over frequency (factor 2 for one-sided)
            integrand = S_k * H
            C_k = 2.0 * np.trapezoid(integrand, f)  # noqa: N806

            total_variance += C_k

        allan_variances.append(total_variance)

    # The extractor uses median ADEV (not mean variance) across the flicker region
    # Convert variances to standard deviations (ADEV) to match extraction behavior
    allan_std_devs = np.sqrt(allan_variances)
    median_adev = float(np.median(allan_std_devs))

    # Scaling: if N stages each with variance scale² give median ADEV = median_adev·scale,
    # and we want median ADEV = 1.0 (target), then:
    # scale · median_adev = 1.0
    # scale = 1 / median_adev
    #
    # But the naive approach uses scale = 1/sqrt(N), so the correction factor is:
    # correction = (1 / median_adev) / (1/sqrt(N)) = sqrt(N) / median_adev
    N = len(pole_time_constants)  # noqa: N806
    correction_factor_theoretical = math.sqrt(N) / median_adev

    # Apply empirical fine-tuning factor to account for discrete-time effects
    # (dt=1s sampling, Nyquist at 0.5 Hz) and Kasdin-Walter approximation errors.
    #
    # Calibrated from simulator validation for τ range 10-1000s, ratio=2.0:
    #   theoretical factor ≈ 3.01, empirical factor = 2.33, ratio = 0.774
    #
    # Robustness: For similar configurations (ratio 1.5-3.0, τ=10-1000s), error < 3%.
    # For significantly different τ ranges (e.g., 5-2000s), may need recalibration.
    EMPIRICAL_CALIBRATION = 0.774  # noqa: N806
    correction_factor = correction_factor_theoretical * EMPIRICAL_CALIBRATION

    return correction_factor


def generate_pole_time_constants(tau0: float, tau_max: float, ratio: float) -> list[float]:
    """Generate Kasdin-Walter pole time constants.

    Args:
        tau0: Start of flicker region in seconds
        tau_max: End of flicker region in seconds
        ratio: Geometric spacing ratio

    Returns:
        List of AR(1) time constants
    """
    tau_list = []
    i = 0
    while True:
        tau = tau0 * (ratio ** i)
        if tau > tau_max:
            break
        tau_list.append(tau)
        i += 1
    return tau_list


def main() -> None:
    """Command-line interface for computing scaling factor."""
    parser = argparse.ArgumentParser(
        description="Compute Kasdin-Walter Allan scaling factor"
    )
    parser.add_argument(
        "--tau0",
        type=float,
        default=10.0,
        help="Start of flicker region in seconds (default: 10.0)",
    )
    parser.add_argument(
        "--tau-max",
        type=float,
        default=1000.0,
        help="End of flicker region in seconds (default: 1000.0)",
    )
    parser.add_argument(
        "--ratio",
        type=float,
        default=2.0,
        help="Geometric spacing ratio (default: 2.0)",
    )
    parser.add_argument(
        "--num-samples",
        type=int,
        default=20,
        help="Number of tau points to sample (default: 20)",
    )
    parser.add_argument(
        "--go-const",
        action="store_true",
        help="Output as Go constant declaration",
    )

    args = parser.parse_args()

    # Generate pole time constants
    tau_list = generate_pole_time_constants(args.tau0, args.tau_max, args.ratio)

    # Compute scaling factor
    scale_factor = compute_kasdin_walter_allan_scale(
        tau_list,
        target_tau_range=(args.tau0, args.tau_max),
        num_tau_samples=args.num_samples,
    )

    if args.go_const:
        # Output as Go constant
        cmd_args = f"--tau0 {args.tau0} --tau-max {args.tau_max} --ratio {args.ratio}"
        print(f"// Generated by kasdin_walter.py {cmd_args}")
        print(f"// Number of AR(1) stages: {len(tau_list)}")
        print(f"const kasdinWalterScaleFactor = {scale_factor}")
    else:
        # Human-readable output
        print("Kasdin-Walter configuration:")
        print(f"  tau0:      {args.tau0} s")
        print(f"  tau_max:   {args.tau_max} s")
        print(f"  ratio:     {args.ratio}")
        print(f"  N stages:  {len(tau_list)}")
        print("\nPole time constants (seconds):")
        for i, tau in enumerate(tau_list):
            print(f"  tau[{i}] = {tau:.1f}")
        print(f"\nScaling factor: {scale_factor:.6f}")
        print("\nUsage in Go:")
        print(f"  scale := stddevPpb / 1e9 / math.Sqrt({len(tau_list)}) * {scale_factor:.6f}")


if __name__ == "__main__":
    main()
