import {
  AmberTransitionError,
  AmberValidationError,
  Provenance,
} from "./provenance.js";
import {
  IncomingInspection,
  IncomingPolicy,
  MAX_INCOMING_JSON_BYTES,
  inspectIncomingJSON,
} from "./incoming.js";

export const PROVENANCE_FIELD = "Amber-Provenance";
export const MAX_ENCODED_VALUE_BYTES = 4 * Math.ceil(MAX_INCOMING_JSON_BYTES / 3);

const BASE64URL_ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_";

/** Encode a provenance value as unpadded base64url of UTF-8 canonical JSON. */
export function encodeValue(provenance: Provenance): string {
  const json = JSON.stringify(provenance);
  const bytes = new TextEncoder().encode(json);
  if (bytes.byteLength > MAX_INCOMING_JSON_BYTES) {
    throw new AmberValidationError(`encoded JSON exceeds ${MAX_INCOMING_JSON_BYTES} bytes`);
  }
  return encodeBase64Url(bytes);
}

/** Decode and inspect a transport value without installing it. */
export function decodeValue(
  value: string | null | undefined,
  policy: IncomingPolicy,
): IncomingInspection {
  validatePolicy(policy);
  if (value === null || value === undefined || value.length === 0) {
    return { present: false };
  }
  if (value.length > MAX_ENCODED_VALUE_BYTES) {
    return invalid(policy, `value exceeds ${MAX_ENCODED_VALUE_BYTES} encoded bytes`);
  }
  let bytes: Uint8Array;
  try {
    bytes = decodeBase64Url(value);
  } catch (error) {
    return invalid(policy, `invalid base64url value: ${String(error)}`);
  }
  try {
    const json = new TextDecoder("utf-8", { fatal: true }).decode(bytes);
    return inspectIncomingJSON(json, policy);
  } catch (error) {
    return invalid(policy, error);
  }
}

function validatePolicy(policy: IncomingPolicy): void {
  if (policy !== "reject" && policy !== "ignore") {
    throw new AmberTransitionError(`unsupported incoming policy ${String(policy)}`);
  }
}

function invalid(policy: IncomingPolicy, error: unknown): IncomingInspection {
  if (policy === "ignore") {
    return { present: false };
  }
  if (error instanceof AmberValidationError) {
    throw error;
  }
  throw new AmberValidationError(String(error));
}

function encodeBase64Url(bytes: Uint8Array): string {
  let output = "";
  for (let index = 0; index < bytes.length; index += 3) {
    const first = bytes[index];
    const second = index + 1 < bytes.length ? bytes[index + 1] : 0;
    const third = index + 2 < bytes.length ? bytes[index + 2] : 0;
    output += BASE64URL_ALPHABET[first >> 2];
    output += BASE64URL_ALPHABET[((first & 0x03) << 4) | (second >> 4)];
    if (index + 1 < bytes.length) {
      output += BASE64URL_ALPHABET[((second & 0x0f) << 2) | (third >> 6)];
    }
    if (index + 2 < bytes.length) {
      output += BASE64URL_ALPHABET[third & 0x3f];
    }
  }
  return output;
}

function decodeBase64Url(value: string): Uint8Array {
  if (!/^[A-Za-z0-9_-]*$/.test(value) || value.length % 4 === 1) {
    throw new Error("invalid unpadded base64url");
  }
  const bytes: number[] = [];
  let buffer = 0;
  let bits = 0;
  for (const character of value) {
    const digit = BASE64URL_ALPHABET.indexOf(character);
    buffer = (buffer << 6) | digit;
    bits += 6;
    if (bits >= 8) {
      bits -= 8;
      bytes.push((buffer >> bits) & 0xff);
      buffer &= (1 << bits) - 1;
    }
  }
  if (bits > 0 && (buffer & ((1 << bits) - 1)) !== 0) {
    throw new Error("non-zero trailing base64url bits");
  }
  return new Uint8Array(bytes);
}
