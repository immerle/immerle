// ponytail: expo/src/winter installs a Hermes-oriented structuredClone ponyfill
// (@ungap/structured-clone) that doesn't support Blob, breaking IndexedDB tests
// that round-trip Blobs (fake-indexeddb calls the global structuredClone). Node's
// real implementation, which handles Blob correctly, is stashed by Expo's own
// polyfill installer before it overwrites the global. If Expo renames/removes
// that backup, this throws loudly in CI rather than silently corrupting Blobs.
if (globalThis.originalstructuredClone) {
  global.structuredClone = globalThis.originalstructuredClone;
}
