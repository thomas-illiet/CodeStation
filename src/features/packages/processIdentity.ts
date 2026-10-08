/** Process identity helpers used by the cross-window operation lock. */
import { promises as fs } from "node:fs";

/** Returns the Linux process start identifier used to detect PID reuse. */
export async function processStartId(pid: number): Promise<string | undefined> {
  try {
    const stat = await fs.readFile(`/proc/${pid}/stat`, "utf8");
    const closingParenthesis = stat.lastIndexOf(")");
    return stat.slice(closingParenthesis + 2).split(" ")[19];
  } catch {
    return undefined;
  }
}
