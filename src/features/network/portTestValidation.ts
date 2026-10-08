/** Validates arguments passed to netcat without invoking a shell. */
export function validPortTarget(host: string, port: number): boolean {
  return host.length > 0 && host.length <= 253 && /^[a-zA-Z0-9._:[\]-]+$/u.test(host) && Number.isInteger(port) && port >= 1 && port <= 65_535;
}
