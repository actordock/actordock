export function decodePtyOutput(value: unknown): Uint8Array {
  if (value instanceof Uint8Array) {
    return value;
  }
  if (Array.isArray(value)) {
    return Uint8Array.from(value);
  }
  if (typeof value === "string") {
    try {
      const binary = atob(value);
      const bytes = new Uint8Array(binary.length);
      for (let i = 0; i < binary.length; i++) {
        bytes[i] = binary.charCodeAt(i);
      }
      return bytes;
    } catch {
      return new TextEncoder().encode(value);
    }
  }
  return new Uint8Array();
}

export function writePtyToTerminal(
  write: (data: string | Uint8Array) => void,
  value: unknown,
): void {
  const bytes = decodePtyOutput(value);
  if (bytes.length === 0) {
    return;
  }
  write(bytes);
}
