import type {FixLevel, SolutionDim} from './gpsprot';

// Display words for the fix vocabulary of NavEpochMsg. The tables contain
// only tokens whose display differs from the serialized name; any other
// token (including ones added later on the Go side) displays as is.

const fixLevelWord: Record<string, string> = {
    notMeasured: 'not-measured',
    carrierFloat: 'float',
    carrierFixed: 'fixed',
};

const solutionDimWord: Record<string, string> = {
    timeOnly: 'time-only',
};

const corrWord: Record<string, string> = {
    used: 'corr',
    partialDualFreq: 'dual-freq (coarse)',
    fullDualFreq: 'dual-freq (precise)',
    PPPConverging: 'PPP converging',
    PPPConverged: 'PPP converged',
};

// fixLevelDisplay returns the display word for a serialized FixLevel.
export function fixLevelDisplay(level: FixLevel): string {
    return fixLevelWord[level] ?? level;
}

// solutionDimDisplay returns the display word for a serialized SolutionDim.
export function solutionDimDisplay(dim: SolutionDim): string {
    return solutionDimWord[dim] ?? dim;
}

// corrDisplay returns the display word for one element of a serialized CorrKind.
export function corrDisplay(name: string): string {
    return corrWord[name] ?? name;
}
