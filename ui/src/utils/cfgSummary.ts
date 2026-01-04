import type { BasicBlockInfo, InstructionInfo } from '../types';

export interface BlockSummary {
  /** Interesting instructions (calls, defers, go statements) */
  instructions: InstructionInfo[];
  /** Formatted terminator text (e.g., "if err != nil", "return") */
  terminator: string | null;
  /** Whether this block is "trivial" (just a jump to next block) */
  isTrivial: boolean;
}

/** Instructions to show in summary view */
const INTERESTING_OPS = new Set(['call', 'defer', 'go', 'panic']);

/**
 * Derives a semantic summary from a basic block.
 * Filters out SSA internals (phi, alloc, store, etc.) and keeps only
 * meaningful operations: calls, defers, go statements, and terminators.
 */
export function deriveBlockSummary(block: BasicBlockInfo): BlockSummary {
  const instructions: InstructionInfo[] = [];
  let terminator: string | null = null;

  for (const instr of block.instructions) {
    // Collect interesting instructions (but not panic - we'll show that as terminator)
    if (INTERESTING_OPS.has(instr.op) && instr.op !== 'panic') {
      instructions.push(instr);
    }
  }

  // Derive terminator text from last instruction
  const lastInstr = block.instructions[block.instructions.length - 1];
  if (lastInstr) {
    switch (lastInstr.op) {
      case 'if':
        // Use the more readable branch_cond if available
        terminator = block.branch_cond ? `if ${block.branch_cond}` : lastInstr.text;
        break;
      case 'return':
        terminator = lastInstr.text;
        break;
      case 'panic':
        terminator = lastInstr.text;
        break;
      case 'jump':
        // Jump with single successor = flow-through, no terminator needed
        terminator = null;
        break;
    }
  }

  // A block is trivial if it has no interesting instructions and just jumps
  const isTrivial =
    instructions.length === 0 &&
    lastInstr?.op === 'jump' &&
    block.successors.length === 1;

  return { instructions, terminator, isTrivial };
}
